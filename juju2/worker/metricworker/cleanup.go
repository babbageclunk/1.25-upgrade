// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"time"

	"github.com/juju/loggo"

	"github.com/juju/1.25-upgrade/juju2/api/metricsmanager"
	"github.com/juju/1.25-upgrade/juju2/worker"
)

var cleanupLogger = loggo.GetLogger("juju.worker.metricworker.cleanup")

const cleanupPeriod = time.Hour

// NewCleanup creates a new periodic worker that calls the CleanupOldMetrics api.
func newCleanup(client metricsmanager.MetricsManagerClient, notify chan string) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			cleanupLogger.Warningf("failed to cleanup %v - will retry later", err)
			return nil
		}
		select {
		case notify <- "cleanupCalled":
		default:
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, cleanupPeriod, worker.NewTimer)
}
