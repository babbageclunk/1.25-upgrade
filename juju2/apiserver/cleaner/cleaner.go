// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The cleaner package implements the API interface
// used by the cleaner worker.

package cleaner

import (
	"github.com/juju/1.25-upgrade/juju2/apiserver/common"
	"github.com/juju/1.25-upgrade/juju2/apiserver/facade"
	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	"github.com/juju/1.25-upgrade/juju2/state"
	"github.com/juju/1.25-upgrade/juju2/state/watcher"
)

func init() {
	common.RegisterStandardFacade("Cleaner", 2, NewCleanerAPI)
}

// CleanerAPI implements the API used by the cleaner worker.
type CleanerAPI struct {
	st        StateInterface
	resources facade.Resources
}

// NewCleanerAPI creates a new instance of the Cleaner API.
func NewCleanerAPI(
	st *state.State,
	res facade.Resources,
	authorizer facade.Authorizer,
) (*CleanerAPI, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &CleanerAPI{
		st:        getState(st),
		resources: res,
	}, nil
}

// Cleanup triggers a state cleanup
func (api *CleanerAPI) Cleanup() error {
	return api.st.Cleanup()
}

// WatchChanges watches for cleanups to be perfomed in state
func (api *CleanerAPI) WatchCleanups() (params.NotifyWatchResult, error) {
	watch := api.st.WatchCleanups()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: api.resources.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{
		Error: common.ServerError(watcher.EnsureErr(watch)),
	}, nil
}
