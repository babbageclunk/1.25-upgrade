// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"

	"github.com/juju/1.25-upgrade/juju2/api/base"
	"github.com/juju/1.25-upgrade/juju2/api/unitassigner"
	"github.com/juju/1.25-upgrade/juju2/cmd/jujud/agent/engine"
	"github.com/juju/1.25-upgrade/juju2/worker"
	"github.com/juju/1.25-upgrade/juju2/worker/dependency"
)

// ManifoldConfig describes the resources used by a unitassigner worker.
type ManifoldConfig engine.APIManifoldConfig

// Manifold returns a Manifold that runs a unitassigner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return engine.APIManifold(
		engine.APIManifoldConfig(config),
		manifoldStart,
	)
}

// manifoldStart returns a unitassigner worker using the supplied APICaller.
func manifoldStart(apiCaller base.APICaller) (worker.Worker, error) {
	facade := unitassigner.New(apiCaller)
	worker, err := New(facade)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}
