package execute

import (
	"bytes"
	"errors"
	"github.com/ms-xy/Tutortool/src/utility/errorutils"
	"github.com/ms-xy/Tutortool/src/utility/slices"
	"io"
	"os/exec"
	"reflect"
	"sync"
	"syscall"
	"time"

	"runtime/debug"

	// logging
	"github.com/ms-xy/logtools"
)

// Helper function to retrieve the exit code
func GetExitCode(err error) int {
	if exitError, ok := err.(*exec.ExitError); ok {
		if state, ok := exitError.Sys().(syscall.WaitStatus); ok {
			return state.ExitStatus()
		}
	}
	return 0
}

/*
Helper functions to deal with program execution
execute returns as following:
stdout, stderr, error
Where error may be of type IOError with potentially embedded errors that are the
original errors (like pipe / fd IO errors).
Other error types may be applicable (like *exec.ExitError) if cmd.Start or
cmd.Wait() fail.
Also at most one error is returned, though multiple may be possible at the same
time. (e.g. IO errors on multiple pipes simultaneously)
*/
type IOError struct {
	Msg         string `json:"msg"`
	OriginalErr error
}

func (err IOError) Error() string {
	if err.OriginalErr != nil {
		return err.Msg + " - " + err.OriginalErr.Error()
	} else {
		return err.Msg
	}
}

var (
	UnknownIOError = IOError{"Unknown Error", nil}
	EOSTDOUT       = errors.New("stdout overflow")
	EOSTDERR       = errors.New("stderr overflow")
	ETIMEOUT       = errors.New("timeout reached")
)

func NewIOError(msg string, original error) IOError {
	return IOError{msg, original}
}

func IsIOError(err error) bool {
	return reflect.TypeOf(err) == reflect.TypeOf(UnknownIOError)
}

func cleanupFDs(list map[string]interface{}) {
	for _, i_fd := range list {
		switch i_fd.(type) {
		case io.ReadCloser:
			i_fd.(io.ReadCloser).Close()
		case io.WriteCloser:
			i_fd.(io.WriteCloser).Close()
		}
	}
}

func Execute(command *Command) (*ExecResult, error) {

	logtools.WithFields(map[string]interface{}{
		"0_path":  command.Executable,
		"1_args":  command.Arguments,
		"2_wdir":  command.WorkingDir,
		"3_input": command.Input,
	}).Debug("received exec command:")

	// evaluate inputs and set sensible defaults where necessary
	if err := command.verify(); err != nil {
		panic(err)
	}

	if command.RlimitArgs != nil && len(command.RlimitArgs) > 0 {
		command.Arguments = slices.StringSlicesJoin(
			command.RlimitArgs,
			[]string{
				"-wdir", command.WorkingDir,
				"-executable", command.Executable,
				"--",
			},
			command.Arguments,
		)
		command.Executable = "./rlimiter"
		command.WorkingDir = "."
	}

	if command.StdoutSize == 0 {
		command.StdoutSize = 10000 // defaulf of 10kB
	}

	if command.StderrSize == 0 {
		command.StderrSize = 10000
	}

	// convenience shortcut variables
	path := command.Executable
	args := command.Arguments
	wdir := command.WorkingDir
	input := command.Input

	logtools.WithFields(map[string]interface{}{
		"0_path":  path,
		"1_args":  args,
		"2_wdir":  wdir,
		"3_input": input,
	}).Debug("actual exec command:")

	// logtools.Debugf(
	// 	"path='%s', args='%v', wdir='%s', input='%v'",
	// 	path, args, wdir, input,
	// )

	// create command
	cmd := exec.Command(path, args...)
	cmd.Dir = wdir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// helpers
	fdlist := make(map[string]interface{})
	defer func() { cleanupFDs(fdlist) }()

	// get stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, NewIOError("Opening stdin pipe failed", err)
	}
	fdlist["stdin"] = stdin

	// get stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, NewIOError("Opening stdout pipe failed", err)
	}
	fdlist["stdout"] = stdout

	// get stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, NewIOError("Opening stderr pipe failed", err)
	}
	fdlist["stderr"] = stderr

	// launch program
	err = cmd.Start()
	if err != nil {
		return nil, NewIOError("Starting program failed", err)
	}

	// process group kill closure
	var (
		isKilled   = false
		killLock   = sync.Mutex{}
		killReason error
	)
	kill := func(reason error) {
		killLock.Lock()
		defer killLock.Unlock()

		if !isKilled {
			killReason = reason
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			if err != nil {
				if errno, ok := err.(syscall.Errno); ok {
					if errno == syscall.ESRCH {
						// ignore
						return
					}
				}
				logtools.Error(err)
				debug.PrintStack()
				// TODO: I think it's quite ok to ignore any error here
				// we can't really do anything about it anyways
				// but we'll still log any error that is out of the ordinary
				// (process not found is to be expected in case there's an error here
				// but the process got killed nonetheless - this function is called
				// thrice after all)
			} else {
				isKilled = true
			}
		}
	}

	// launch goroutines for feeding and for reading
	// todo controller goroutine that checks resource consumption
	var (
		wg          = &sync.WaitGroup{}
		stdoutBytes []byte
		stderrBytes []byte
		stdinError  error
		stdoutError error
		stderrError error
	)
	wg.Add(3)
	go func() {
		defer func() {
			stdin.Close()
			delete(fdlist, "stdin")
		}()
		defer wg.Done()
		stdinError = feedFD(input, stdin)
	}()
	go func() {
		defer wg.Done()
		stdoutBytes, stdoutError = readFD(stdout, command.StdoutSize, EOSTDOUT)
		if stdoutError != nil {
			kill(stdoutError)
		}
	}()
	go func() {
		defer wg.Done()
		stderrBytes, stderrError = readFD(stderr, command.StderrSize, EOSTDERR)
		if stderrError != nil {
			kill(stderrError)
		}
	}()

	// kill after timeout if configured
	if command.Timeout > 0 {
		time.AfterFunc(command.Timeout, func() { kill(ETIMEOUT) })
	}

	// wait for results
	programError := cmd.Wait()
	exitCode := GetExitCode(programError)
	// if programError.(exec.ExitError).Error() == "signal: killed" {}
	wg.Wait()

	logtools.WithFields(map[string]interface{}{
		"0_stdout": formatOutput(clip(string(stdoutBytes), 100)),
		"1_stderr": formatOutput(clip(string(stderrBytes), 100)),
		"2_error":  programError,
	}).Debug("command result:")

	// logtools.Debugf(
	// 	"error='%v', stdout='%s', stderr='%s'",
	// 	programError, string(stdoutBytes), string(stderrBytes))

	err = nil
	if programError != nil {
		err = programError
	} else if stdinError != nil {
		err = stdinError
	} else if stdoutError != nil {
		err = stdoutError
	} else if stderrError != nil {
		err = stderrError
	}

	result := &ExecResult{
		ExitCode: exitCode,
		Stdout:   stdoutBytes,
		Stderr:   stderrBytes,
		Error:    errorutils.GetString(err),
		ModTime:  time.Now(),
		Killed:   isKilled,
	}
	if isKilled {
		result.KillReason = killReason.Error()
	}

	return result, err
}

func feedFD(input []byte, writer io.WriteCloser) error {
	_, err := io.Copy(writer, bytes.NewBuffer(input))
	return err
}

func readFD(reader io.ReadCloser, maxSize int, EOVER error) ([]byte, error) {
	var (
		data   = make([]byte, maxSize)
		index  = 0
		buffer = make([]byte, 1000) // read buffer of size 1KB
		n      int
		err    error
		result []byte
	)
	for {
		n, err = reader.Read(buffer)
		if n > 0 {
			if index+n > len(data) {
				copy(data[index:], buffer)
				return data, EOVER
			}
			copy(data[index:index+n], buffer[0:n])
			index += n
		}
		if err == io.EOF || err == io.ErrClosedPipe || err == io.ErrUnexpectedEOF {
			err = nil
			break
		} else if err != nil {
			logtools.Log("Unexpected error reading from pipe:", err)
			break // error happened it seems, so lets abort
		}
	}
	result = make([]byte, index)
	copy(result, data)
	return result, err
}
