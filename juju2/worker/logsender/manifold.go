// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsender

import (
	"github.com/juju/1.25-upgrade/juju2/api/base"
	"github.com/juju/1.25-upgrade/juju2/api/logsender"
	"github.com/juju/1.25-upgrade/juju2/cmd/jujud/agent/engine"
	"github.com/juju/1.25-upgrade/juju2/worker"
	"github.com/juju/1.25-upgrade/juju2/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	APICallerName string
	LogSource     LogRecordCh
}

// Manifold returns a dependency manifold that runs a logger
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.APIManifoldConfig{
		APICallerName: config.APICallerName,
	}
	return engine.APIManifold(typedConfig, config.newWorker)
}

func (config ManifoldConfig) newWorker(apiCaller base.APICaller) (worker.Worker, error) {
	return New(config.LogSource, logsender.NewAPI(apiCaller)), nil
}
