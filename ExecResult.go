package execute

import (
	"time"
)

// convenience structure for return values
// directly used as parts for the db-models of RunResult and GccResult
type ExecResult struct {
	Stdout     []byte
	Stderr     []byte
	ExitCode   int
	Error      string
	ModTime    time.Time
	Killed     bool
	KillReason string
}
