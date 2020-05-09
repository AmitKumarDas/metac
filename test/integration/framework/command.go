package framework

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

// Command is a wrapper around the exec.Command invocation.
type Command struct {
	// Out, Err specify where Command should write its StdOut &
	// StdErr to.
	//
	// If not specified, the output will be discarded in case of
	// no errors or added to error details in case of errors.
	Out io.Writer
	Err io.Writer

	// out & err buffers are used to provide additional details
	// in case of errors. These buffers are used only if Out & Err
	// are not set by the callers of Command.
	outBuf *bytes.Buffer
	errBuf *bytes.Buffer
}

// CommandConfig is used to create a new instance of Command
type CommandConfig struct {
	Out io.Writer
	Err io.Writer
}

// NewCommand returns a new instance of Command
func NewCommand(config CommandConfig) *Command {
	cmd := &Command{
		Out: config.Out,
		Err: config.Err,
	}
	if cmd.Out == nil {
		cmd.outBuf = new(bytes.Buffer)
	}
	if cmd.Err == nil {
		cmd.errBuf = new(bytes.Buffer)
	}
	return cmd
}

// wrapError wraps the given error with additional information
func (c *Command) wrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	var details []string
	details = append(
		details,
		fmt.Sprintf(format, args...),
	)
	if c.errBuf != nil {
		errstr := c.errBuf.String()
		if errstr != "" {
			details = append(details, errstr)
		}
	}
	if c.outBuf != nil {
		outstr := c.outBuf.String()
		if outstr != "" {
			details = append(details, outstr)
		}
	}
	return errors.Wrapf(
		err,
		strings.Join(details, ": "),
	)
}

// build returns a new instance of exec.Cmd
func (c *Command) build(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	if c.Out != nil {
		cmd.Stdout = c.Out
	} else if c.outBuf != nil {
		cmd.Stdout = c.outBuf
	}
	if c.Err != nil {
		cmd.Stderr = c.Err
	} else if c.errBuf != nil {
		cmd.Stderr = c.errBuf
	}
	return cmd
}

// Run executes the given command along with its arguments
func (c *Command) Run(name string, args ...string) error {
	cmd := c.build(name, args...)
	return c.wrapErrorf(
		cmd.Run(),
		"Failed to run %s",
		cmd.Args,
	)
}

// Start starts the given command and returns corresponding
// stop function. Start does not wait for the command to
// complete.
func (c *Command) Start(name string, args ...string) (func() error, error) {
	cmd := c.build(name, args...)
	err := cmd.Start()
	if err != nil {
		return nil, c.wrapErrorf(
			err,
			"Failed to start %s",
			cmd.Args,
		)
	}
	return func() error {
		err := cmd.Process.Signal(os.Interrupt)
		if err != nil {
			return c.wrapErrorf(
				err,
				"Failed to stop %s",
				cmd.Args,
			)
		}
		return c.wrapErrorf(
			cmd.Wait(),
			"Failed to stop: Wait failed: %s",
			cmd.Args,
		)
	}, nil
}
