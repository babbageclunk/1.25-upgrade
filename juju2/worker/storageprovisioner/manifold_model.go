// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/api/base"
	"github.com/juju/1.25-upgrade/juju2/api/storageprovisioner"
	"github.com/juju/1.25-upgrade/juju2/environs"
	"github.com/juju/1.25-upgrade/juju2/worker"
	"github.com/juju/1.25-upgrade/juju2/worker/dependency"
)

// ModelManifoldConfig defines a storage provisioner's configuration and dependencies.
type ModelManifoldConfig struct {
	APICallerName string
	ClockName     string
	EnvironName   string

	Scope      names.Tag
	StorageDir string
}

// ModelManifold returns a dependency.Manifold that runs a storage provisioner.
func ModelManifold(config ModelManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.ClockName, config.EnvironName},
		Start: func(context dependency.Context) (worker.Worker, error) {

			var clock clock.Clock
			if err := context.Get(config.ClockName, &clock); err != nil {
				return nil, errors.Trace(err)
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}
			var environ environs.Environ
			if err := context.Get(config.EnvironName, &environ); err != nil {
				return nil, errors.Trace(err)
			}

			api, err := storageprovisioner.NewState(apiCaller, config.Scope)
			if err != nil {
				return nil, errors.Trace(err)
			}
			w, err := NewStorageProvisioner(Config{
				Scope:       config.Scope,
				StorageDir:  config.StorageDir,
				Volumes:     api,
				Filesystems: api,
				Life:        api,
				Registry:    environ,
				Machines:    api,
				Status:      api,
				Clock:       clock,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
