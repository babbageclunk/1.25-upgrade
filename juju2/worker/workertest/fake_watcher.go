// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workertest

import (
	"errors"

	"github.com/juju/1.25-upgrade/juju2/worker"
)

type NotAWatcher struct {
	changes chan struct{}
	worker.Worker
}

// NewFakeWatcher returns a fake watcher
func NewFakeWatcher(len, preload int) NotAWatcher {
	if len < preload {
		panic("len must be larger than preload")
	}
	ch := make(chan struct{}, len)
	for i := 0; i < preload; i++ {
		ch <- struct{}{}
	}
	return NotAWatcher{
		changes: ch,
		Worker:  NewErrorWorker(nil),
	}
}

func (w NotAWatcher) Changes() <-chan struct{} {
	return w.changes
}

func (w NotAWatcher) Stop() error {
	return nil
}

func (w NotAWatcher) Err() error {
	return errors.New("An error")
}

func (w *NotAWatcher) Ping() {
	w.changes <- struct{}{}
}

func (w *NotAWatcher) Close() {
	close(w.changes)
}
