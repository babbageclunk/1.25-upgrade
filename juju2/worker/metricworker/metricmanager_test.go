// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/1.25-upgrade/juju2/testing"
	"github.com/juju/1.25-upgrade/juju2/worker/metricworker"
)

type MetricManagerSuite struct{}

var _ = gc.Suite(&MetricManagerSuite{})

func (s *MetricManagerSuite) TestRunner(c *gc.C) {
	notify := make(chan string, 2)
	var client mockClient
	_, err := metricworker.NewMetricsManager(&client, notify)
	c.Assert(err, jc.ErrorIsNil)
	expectedCalls := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case call := <-notify:
			expectedCalls[call] = true
		case <-time.After(coretesting.LongWait):
			c.Logf("we should have received a notification by now")
		}
	}

	c.Check(expectedCalls["senderCalled"], jc.IsTrue)
	c.Check(expectedCalls["cleanupCalled"], jc.IsTrue)
}
