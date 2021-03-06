// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/1.25-upgrade/juju2/juju/osenv"
	"github.com/juju/1.25-upgrade/juju2/provider/dummy"
	coretesting "github.com/juju/1.25-upgrade/juju2/testing"
)

// FlagRunMain is used to indicate that the -run-main flag was used.
var FlagRunMain = flag.Bool("run-main", false, "Run the application's main function for recursive testing")

// BadRun is used to run a command, check the exit code, and return the output.
func BadRun(c *gc.C, exit int, args ...string) string {
	localArgs := append([]string{"-test.run", "TestRunMain", "-run-main", "--"}, args...)
	ps := exec.Command(os.Args[0], localArgs...)
	ps.Env = append(os.Environ(), osenv.JujuXDGDataHomeEnvKey+"="+osenv.JujuXDGDataHome())
	output, err := ps.CombinedOutput()
	c.Logf("command output: %q", output)
	if exit != 0 {
		c.Assert(err, gc.ErrorMatches, fmt.Sprintf("exit status %d", exit))
	}
	return string(output)
}

// HelpText returns a command's formatted help text.
func HelpText(command cmd.Command, name string) string {
	buff := &bytes.Buffer{}
	info := command.Info()
	info.Name = name
	f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
	command.SetFlags(f)
	buff.Write(info.Help(f))
	return buff.String()
}

type gcWriter struct {
	c      *gc.C
	source string
}

func (w *gcWriter) Write(p []byte) (n int, err error) {
	message := fmt.Sprintf("%s: %s", w.source, p)
	// Magic calldepth value...
	// The value says "how far up the call stack do we go to find the location".
	// It is used to match the standard library log function, and isn't actually
	// used by gocheck.
	w.c.Output(3, message)
	return len(p), nil
}

// NullContext returns a no-op command context.
func NullContext(c *gc.C) *cmd.Context {
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdin = io.LimitReader(nil, 0)
	ctx.Stdout = &gcWriter{c: c, source: "stdout"}
	ctx.Stderr = &gcWriter{c: c, source: "stderr"}
	return ctx
}

// RunCommand runs the command and returns channels holding the
// command's operations and errors.
func RunCommand(ctx *cmd.Context, com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	if ctx == nil {
		panic("ctx == nil")
	}
	errc = make(chan error, 1)
	opc = make(chan dummy.Operation, 200)
	dummy.Listen(opc)
	go func() {
		defer func() {
			// signal that we're done with this ops channel.
			dummy.Listen(nil)
			// now that dummy is no longer going to send ops on
			// this channel, close it to signal to test cases
			// that we are done.
			close(opc)
		}()

		if err := coretesting.InitCommand(com, args); err != nil {
			errc <- err
			return
		}

		errc <- com.Run(ctx)
	}()
	return
}
