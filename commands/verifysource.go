// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/description"
	"github.com/juju/errors"
)

var verifySourceDoc = `
The purpose of the verify-source command is to check connectivity, status, and
viability of a 1.25 juju environment for migration into a Juju 2.x controller.

`

func newVerifySourceCommand() cmd.Command {
	command := &verifySourceCommand{}
	command.remoteCommand = "verify-source-impl"
	return command
}

type verifySourceCommand struct {
	baseClientCommand
}

func (c *verifySourceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "verify-source",
		Args:    "<environment name>",
		Purpose: "check a 1.25 environment for migration suitability",
		Doc:     verifySourceDoc,
	}
}

func (c *verifySourceCommand) Init(args []string) error {
	args, err := c.baseClientCommand.init(args)
	if err != nil {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args)
}

var verifySourceImplDoc = `

verify-source-impl must be executed on an API server machine of a 1.25
environment.

The command will check the export of the environment into the 2.0 model
format.

`

func newVerifySourceImplCommand() cmd.Command {
	return &verifySourceImplCommand{}
}

type verifySourceImplCommand struct {
	baseRemoteCommand
}

func (c *verifySourceImplCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "verify-source-impl",
		Purpose: "check the database export for migration suitability",
		Doc:     verifySourceImplDoc,
	}
}

func (c *verifySourceImplCommand) Run(ctx *cmd.Context) error {
	st, err := c.getState(ctx)
	if err != nil {
		return errors.Annotate(err, "getting state")
	}
	defer st.Close()

	model, err := st.Export()
	if err != nil {
		return errors.Annotate(err, "exporting model representation")
	}

	// Check for LXC containers
	bytes, err := description.Serialize(model)
	if err != nil {
		return errors.Annotate(err, "serializing model representation")
	}

	_, err = ctx.GetStdout().Write(bytes)
	if err != nil {
		return errors.Annotate(err, "writing model representation")
	}

	return nil
}
