# restart
I simple tool for auto restarting an executable on failure with a backup executable:

* Install with `go install github.com/jptrs93/restart@main`
* Execute with a primary command and an optional secondary command for example: 
  * `restart sleep 10 --- sleep 100`
  * `restart sleep 1 --- ../../venv/bin/python -c "import time; time.sleep(10); raise Exception('random error')"`
* If you kill the restart process it will kill the processes of the command it has started. Unless you run the the `--child-detach` flag in which case it wont.
