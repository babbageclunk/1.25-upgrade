// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package model

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/api/modelmanager"
	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	"github.com/juju/1.25-upgrade/juju2/cmd/juju/common"
	"github.com/juju/1.25-upgrade/juju2/cmd/modelcmd"
	"github.com/juju/1.25-upgrade/juju2/cmd/output"
	"github.com/juju/1.25-upgrade/juju2/jujuclient"
)

const showModelCommandDoc = `Show information about the current or specified model`

func NewShowCommand() cmd.Command {
	showCmd := &showModelCommand{}
	showCmd.RefreshModels = showCmd.ModelCommandBase.RefreshModels
	return modelcmd.Wrap(showCmd, modelcmd.WrapSkipModelFlags)
}

// showModelCommand shows all the users with access to the current model.
type showModelCommand struct {
	modelcmd.ModelCommandBase
	// RefreshModels hides the RefreshModels function defined
	// in ModelCommandBase. This allows overriding for testing.
	// NOTE: ideal solution would be to have the base implement a method
	// like store.ModelByName which auto-refreshes.
	RefreshModels func(jujuclient.ClientStore, string) error

	out cmd.Output
	api ShowModelAPI
}

// ShowModelAPI defines the methods on the client API that the
// users command calls.
type ShowModelAPI interface {
	Close() error
	ModelInfo([]names.ModelTag) ([]params.ModelInfoResult, error)
}

func (c *showModelCommand) getAPI() (ShowModelAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(api), nil
}

// Info implements Command.Info.
func (c *showModelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-model",
		Args:    "<model name>",
		Purpose: "Shows information about the current or specified model.",
		Doc:     showModelCommandDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *showModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements Command.Init.
func (c *showModelCommand) Init(args []string) error {
	if len(args) > 0 {
		c.SetModelName(args[0])
		args = args[1:]
	}
	if err := c.ModelCommandBase.Init(args); err != nil {
		return err
	}
	if c.ModelName() == "" {
		defaultModel, err := modelcmd.GetCurrentModel(c.ClientStore())
		if err != nil {
			return err
		}
		c.SetModelName(defaultModel)
	}
	return nil
}

// Run implements Command.Run.
func (c *showModelCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.getAPI()
	if err != nil {
		return err
	}
	defer api.Close()

	store := c.ClientStore()
	modelDetails, err := store.ModelByName(
		c.ControllerName(),
		c.ModelName(),
	)
	if errors.IsNotFound(err) {
		if err := c.RefreshModels(store, c.ControllerName()); err != nil {
			return errors.Annotate(err, "refreshing models cache")
		}
		// Now try again.
		modelDetails, err = store.ModelByName(
			c.ControllerName(),
			c.ModelName(),
		)
	}
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	modelTag := names.NewModelTag(modelDetails.ModelUUID)
	results, err := api.ModelInfo([]names.ModelTag{modelTag})
	if err != nil {
		return err
	}
	if results[0].Error != nil {
		return results[0].Error
	}
	infoMap, err := c.apiModelInfoToModelInfoMap([]params.ModelInfo{*results[0].Result})
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, infoMap)
}

func (c *showModelCommand) apiModelInfoToModelInfoMap(modelInfo []params.ModelInfo) (map[string]common.ModelInfo, error) {
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	output := make(map[string]common.ModelInfo)
	for _, info := range modelInfo {
		out, err := common.ModelInfoFromParams(info, now)
		if err != nil {
			return nil, errors.Trace(err)
		}
		out.ControllerName = c.ControllerName()
		output[out.Name] = out
	}
	return output, nil
}
