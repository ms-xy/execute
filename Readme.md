## execute

This is a convenience wrapper around the standard GO `exec` package.

It provides a somewhat elegant way to access stdout and stderr and feed
data to stdin, without bothering the programmer with pipes and I/O.

There's little doubt that it can be optimized. It works fine, however, so
there's little incentive to change anything for the better right now.

## Usage

```shell
go get github.com/ms-xy/execute
```

```go
import (
  "github.com/ms-xy/execute"
  "github.com/ms-xy/logtools"
)

func main() {
  logtools.Initialize()

  cmd := &execute.Command{
    LookupPath: true,
    WorkingDir: ".",

    Executable: "bash",
    Arguments:  []string{"-c", '"echo \"hello world\""'},
    Input:      []byte{},

    RlimitArgs: []string{}, // see github.com/ms-xy/rlimiter
    Timeout:    10*time.Second,
    StdoutSize: 10000, // size in bytes
    StderrSize: 10000, // size in bytes
  }

  if result, err := execute.Execute(cmd); err != nil {
    logtools.Errorf("Error happened while executing command: %+v", err)
  } else {
    logtools.Infof("Stdout of command: %s", string(result.Stdout))
  }
}
```

## struct Command
```go
type Command struct {
  LookupPath bool
  Executable string
  WorkingDir string
  Input      []byte
  RlimitArgs []string
  Arguments  []string
  Timeout    time.Duration
  StdoutSize int
  StderrSize int
}
```

## struct ExecResult
```go
type ExecResult struct {
  Stdout     []byte
  Stderr     []byte
  ExitCode   int
  Error      string
  ModTime    time.Time
  Killed     bool
  KillReason string
}
```

## License

GNU GPLv3. Please see the attached License.txt file for details.
Different license terms can be arranged on request.
