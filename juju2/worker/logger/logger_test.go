// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger_test

import (
	"time"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/agent"
	apilogger "github.com/juju/1.25-upgrade/juju2/api/logger"
	"github.com/juju/1.25-upgrade/juju2/juju/testing"
	"github.com/juju/1.25-upgrade/juju2/state"
	"github.com/juju/1.25-upgrade/juju2/worker"
	"github.com/juju/1.25-upgrade/juju2/worker/logger"
)

// worstCase is used for timeouts when timing out
// will fail the test. Raising this value should
// not affect the overall running time of the tests
// unless they fail.
const worstCase = 5 * time.Second

type LoggerSuite struct {
	testing.JujuConnSuite

	loggerAPI *apilogger.State
	machine   *state.Machine
}

var _ = gc.Suite(&LoggerSuite{})

func (s *LoggerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	apiConn, machine := s.OpenAPIAsNewMachine(c)
	// Create the machiner API facade.
	s.loggerAPI = apilogger.NewState(apiConn)
	c.Assert(s.loggerAPI, gc.NotNil)
	s.machine = machine
}

func (s *LoggerSuite) waitLoggingInfo(c *gc.C, expected string) {
	timeout := time.After(worstCase)
	for {
		select {
		case <-timeout:
			c.Fatalf("timeout while waiting for logging info to change")
		case <-time.After(10 * time.Millisecond):
			loggerInfo := loggo.LoggerInfo()
			if loggerInfo != expected {
				c.Logf("logging is %q, still waiting", loggerInfo)
				continue
			}
			return
		}
	}
}

type mockConfig struct {
	agent.Config
	c   *gc.C
	tag names.Tag
}

func (mock *mockConfig) Tag() names.Tag {
	return mock.tag
}

func agentConfig(c *gc.C, tag names.Tag) *mockConfig {
	return &mockConfig{c: c, tag: tag}
}

func (s *LoggerSuite) makeLogger(c *gc.C) (worker.Worker, *mockConfig) {
	config := agentConfig(c, s.machine.Tag())
	w, err := logger.NewLogger(s.loggerAPI, config)
	c.Assert(err, jc.ErrorIsNil)
	return w, config
}

func (s *LoggerSuite) TestRunStop(c *gc.C) {
	loggingWorker, _ := s.makeLogger(c)
	c.Assert(worker.Stop(loggingWorker), gc.IsNil)
}

func (s *LoggerSuite) TestInitialState(c *gc.C) {
	config, err := s.State.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expected := config.LoggingConfig()

	initial := "<root>=DEBUG;wibble=ERROR"
	c.Assert(expected, gc.Not(gc.Equals), initial)

	loggo.DefaultContext().ResetLoggerLevels()
	err = loggo.ConfigureLoggers(initial)
	c.Assert(err, jc.ErrorIsNil)

	loggingWorker, _ := s.makeLogger(c)
	defer worker.Stop(loggingWorker)

	s.waitLoggingInfo(c, expected)
}
