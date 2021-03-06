// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/1.25-upgrade/juju2/api/base"
	apiwatcher "github.com/juju/1.25-upgrade/juju2/api/watcher"
	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	"github.com/juju/1.25-upgrade/juju2/environs/config"
	"github.com/juju/1.25-upgrade/juju2/logfwd/syslog"
	"github.com/juju/1.25-upgrade/juju2/watcher"
)

// ModelWatcher provides common client-side API functions
// to call into apiserver.common.ModelWatcher.
type ModelWatcher struct {
	facade base.FacadeCaller
}

// NewModelWatcher creates a ModelWatcher on the specified facade,
// and uses this name when calling through the caller.
func NewModelWatcher(facade base.FacadeCaller) *ModelWatcher {
	return &ModelWatcher{facade}
}

// WatchForModelConfigChanges return a NotifyWatcher waiting for the
// model configuration to change.
func (e *ModelWatcher) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := e.facade.FacadeCall("WatchForModelConfigChanges", nil, &result)
	if err != nil {
		return nil, err
	}
	return apiwatcher.NewNotifyWatcher(e.facade.RawAPICaller(), result), nil
}

// ModelConfig returns the current model configuration.
func (e *ModelWatcher) ModelConfig() (*config.Config, error) {
	var result params.ModelConfigResult
	err := e.facade.FacadeCall("ModelConfig", nil, &result)
	if err != nil {
		return nil, err
	}
	conf, err := config.New(config.NoDefaults, result.Config)
	if err != nil {
		return nil, err
	}
	return conf, nil
}

// WatchForLogForwardConfigChanges return a NotifyWatcher waiting for the
// log forward syslog configuration to change.
func (e *ModelWatcher) WatchForLogForwardConfigChanges() (watcher.NotifyWatcher, error) {
	// TODO(wallyworld) - lp:1602237 - this needs to have it's own backend implementation.
	// For now, we'll piggyback off the ModelConfig API.
	return e.WatchForModelConfigChanges()
}

// LogForwardConfig returns the current log forward syslog configuration.
func (e *ModelWatcher) LogForwardConfig() (*syslog.RawConfig, bool, error) {
	// TODO(wallyworld) - lp:1602237 - this needs to have it's own backend implementation.
	// For now, we'll piggyback off the ModelConfig API.
	modelConfig, err := e.ModelConfig()
	if err != nil {
		return nil, false, err
	}
	cfg, ok := modelConfig.LogFwdSyslog()
	return cfg, ok, nil
}
