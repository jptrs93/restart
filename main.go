package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func init() {
	runtime.GOMAXPROCS(1) // this program is lightweight it should only ever need 1 os thread
}

const maxRestartsPerHour = 3 // Set the maximum number of restarts within an hour
var restartTimes []time.Time

func main() {
	childDetach := flag.Bool("child-detach", false, "leave the child process running when the parent exits")
	flag.Parse()

	if *childDetach {
		slog.Info("running with -child-detach flag set")
	}

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("Usage: restarter <args> <primary_cmd> <primary_args> --- <backup_cmd> <backup_args>")
		os.Exit(1)
	}
	separatorIndex := -1
	for i, arg := range args {
		if arg == "---" {
			separatorIndex = i
			break
		}
	}

	cmd := args
	var backupCmd []string

	if separatorIndex != -1 && separatorIndex < len(args)-1 {
		cmd = args[:separatorIndex]
		backupCmd = args[separatorIndex+1:]
		verifyExecutablesExists(cmd[0], backupCmd[0])
	} else {
		verifyExecutablesExists(cmd[0])
	}
	var command *exec.Cmd

	// we will stop the process if the restarter process stopped
	sigChan := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(context.Background())
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		slog.Info(fmt.Sprintf("received signal: %v", sig))
		if *childDetach {
			os.Exit(0)
		}
		if command != nil && command.Process != nil {
			slog.Info("restarter process killed, cleaning up (forwarding signal to child process)")
			// forward the signal to the child process
			if err := command.Process.Signal(sig); err != nil {
				slog.Error(fmt.Sprintf("failed to forward signal to child process: %v", err))
			}
		}
		cancel()
		os.Exit(0)
	}()

	for ctx.Err() == nil {
		if len(restartTimes) > 0 && time.Since(restartTimes[len(restartTimes)-1]) < time.Second*10 {
			slog.Info("last restart <10s ago, buffering for 1s")
			time.Sleep(1 * time.Second)
		}
		if tooManyRestartsInHour() && len(backupCmd) > 0 {
			slog.Warn(fmt.Sprintf("too many recent restarts using backup process command instead of primary one"))
			cmd = backupCmd
		}
		restartTimes = append(restartTimes, time.Now())
		slog.Info(fmt.Sprintf("running command: %s\n", strings.Join(cmd, " ")))
		command = exec.Command(cmd[0], cmd[1:]...)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if *childDetach {
			// Detach the child process
			command.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
			}
		}

		err := command.Start()
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to start process: %v", err))
			continue
		}

		// Wait for the command to finish
		err = command.Wait()
		slog.Warn(fmt.Sprintf("process died with exit code %v: %v", command.ProcessState.ExitCode(), err))
	}
}

func tooManyRestartsInHour() bool {
	now := time.Now()
	for i := 0; i < len(restartTimes); {
		if now.Sub(restartTimes[i]) > time.Hour {
			restartTimes = append(restartTimes[:i], restartTimes[i+1:]...)
		} else {
			i++
		}
	}
	return len(restartTimes) > maxRestartsPerHour
}

func verifyExecutablesExists(fp ...string) {
	for _, f := range fp {
		if !executableExists(f) {
			fmt.Println(fmt.Sprintf("no such executable: %v", f))
			os.Exit(1)
		}
	}
}

func executableExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
