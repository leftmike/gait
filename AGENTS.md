# gait

## Build & Test Commands
All Go commands must be run with `go -C /home/mike/gait`
```bash
go -C /home/mike/gait build ./...
go -C /home/mike/gait test -v ./...
go -C /home/mike/gait test -v ./model/
```

## Critical
Never run `go` commands without `-C /home/mike/gait`.

