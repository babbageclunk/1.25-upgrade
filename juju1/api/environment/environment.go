// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/1.25-upgrade/juju1/api/base"
	"github.com/juju/1.25-upgrade/juju1/api/common"
)

const apiName = "Environment"

// Facade provides access to a machine environment worker's view of the world.
type Facade struct {
	*common.EnvironWatcher
	*ToolsVersionUpdater
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{
		EnvironWatcher:      common.NewEnvironWatcher(facadeCaller),
		ToolsVersionUpdater: NewToolsVersionUpdater(facadeCaller),
	}
}
