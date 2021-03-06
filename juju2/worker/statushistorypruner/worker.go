// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/1.25-upgrade/juju2/worker"
)

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(time.Duration, int) error
}

// Config holds all necessary attributes to start a pruner worker.
type Config struct {
	Facade         Facade
	MaxHistoryTime time.Duration
	MaxHistoryMB   uint
	PruneInterval  time.Duration
	// TODO(fwereade): 2016-03-17 lp:1558657
	NewTimer worker.NewTimerFunc
}

// Validate will err unless basic requirements for a valid
// config are met.
func (c *Config) Validate() error {
	if c.Facade == nil {
		return errors.New("missing Facade")
	}
	if c.NewTimer == nil {
		return errors.New("missing Timer")
	}
	// TODO(perrito666) this assumes out of band knowledge of how filter
	// values are treated, expand config to support the "dont use this filter"
	// case as an explicit statement.
	if c.MaxHistoryMB <= 0 && c.MaxHistoryTime <= 0 {
		return errors.New("missing prune criteria, no size or date limit provided")
	}
	return nil
}

// New returns a worker.Worker for history Pruner.
func New(conf Config) (worker.Worker, error) {
	if err := conf.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	doPruning := func(stop <-chan struct{}) error {
		err := conf.Facade.Prune(conf.MaxHistoryTime, int(conf.MaxHistoryMB))
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	return worker.NewPeriodicWorker(doPruning, conf.PruneInterval, conf.NewTimer), nil
}
