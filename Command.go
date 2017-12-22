package execute

import (
	"errors"
	"github.com/mattn/go-zglob"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ms-xy/logtools"
)

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

var (
	ERR_WORKDIR_MISSING  = errors.New("working directory is missing")
	ERR_WORKDIR_ISFILE   = errors.New("working directory must be a directory")
	ERR_EXECUTABLE_ISDIR = errors.New("executable must be a file")
	ERR_NOTEXECUTABLE    = errors.New("file is not executable")
)

func (c *Command) verify() error {
	// check paths
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if !filepath.IsAbs(c.WorkingDir) {
		c.WorkingDir = filepath.Join(cwd, c.WorkingDir)
	}

	if !filepath.IsAbs(c.Executable) {
		path := filepath.Join(c.WorkingDir, c.Executable)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if path, err := exec.LookPath(c.Executable); err != nil {
				return err
			} else {
				c.Executable = path
			}
		} else if err != nil {
			return err
		} else {
			c.Executable = path
		}
	}

	// check if executable exists and is actually executable
	if f, err := os.Stat(c.Executable); err != nil {
		return err

	} else if f.IsDir() {
		return ERR_EXECUTABLE_ISDIR

	} else if (f.Mode() & 0111) == 0 {
		return ERR_NOTEXECUTABLE
	}

	// check if working directory exists and is actually a directory
	if f, err := os.Stat(c.WorkingDir); os.IsNotExist(err) {
		return ERR_WORKDIR_MISSING

	} else if err != nil {
		return err

	} else if !f.IsDir() {
		return ERR_WORKDIR_ISFILE
	}

	// globbing the arguments, this isn't a perfect approach, it ignores quite
	// a bit of problems with evaluating arguments this late
	if err = os.Chdir(c.WorkingDir); err != nil {
		return err
	}
	defer os.Chdir(cwd)

	args := make([]string, 0, len(c.Arguments))
	for _, arg := range c.Arguments {
		matches, _ := zglob.Glob(arg)
		logtools.Debugf("glob='%s', matches='%v'", arg, matches)
		if len(matches) > 0 {
			args = append(args, matches...)
		} else {
			args = append(args, arg)
		}
	}
	c.Arguments = args

	return nil
}
