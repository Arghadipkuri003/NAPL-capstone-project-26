# CPU Scheduling Simulator (Go)

A single-binary CPU scheduling simulator with a web GUI, REST API and SQLite history.

**Algorithms:** FCFS · SJF (non-preemptive) · SRTF · Priority (non-preemptive & preemptive) · Round Robin  
**Metrics:** CT, TAT, WT, RT, averages, CPU utilisation, throughput, idle-time handling  
**Extras:** animated Gantt chart, compare-all view, CSV export, saved runs in SQLite

## Run
```bash
go mod tidy          # first time only
go run .             # http://127.0.0.1:8080
go run . -addr :9000 -db my.db
go test ./...
```
The SQLite driver (`mattn/go-sqlite3`) needs a C compiler (gcc / MinGW-w64 on Windows).
To avoid cgo, swap the import in `store/store.go` for `modernc.org/sqlite` and use driver name `"sqlite"`.

## Structure
```
main.go                 HTTP server, routes, embedded GUI
scheduler/scheduler.go  algorithms, validation, metrics, CSV writer
scheduler/*_test.go     hand-verified test cases
store/store.go          SQLite persistence
web/index.html          GUI (vanilla JS, no build step)
```

## API
| Method | Path | Purpose |
|---|---|---|
| POST | /api/simulate | run one algorithm |
| POST | /api/compare | run all algorithms |
| POST | /api/csv | download CSV report |
| POST/GET | /api/runs | save / list runs |
| GET/DELETE | /api/runs/{id} | load / delete a run |

Body: `{"algorithm":"rr","quantum":2,"processes":[{"pid":"P1","arrival":0,"burst":5,"priority":1}]}`

## Conventions
Integer time units, single CPU, lower priority number = higher priority. Ties: earlier arrival, then input order.
In Round Robin, processes arriving during a slice queue ahead of the preempted process.
