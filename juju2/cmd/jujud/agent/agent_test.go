// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/series"
	gc "gopkg.in/check.v1"

	"github.com/juju/1.25-upgrade/juju2/cmd/jujud/agent/agenttest"
	cmdutil "github.com/juju/1.25-upgrade/juju2/cmd/jujud/util"
	imagetesting "github.com/juju/1.25-upgrade/juju2/environs/imagemetadata/testing"
	"github.com/juju/1.25-upgrade/juju2/juju/paths"
	"github.com/juju/1.25-upgrade/juju2/mongo"
	"github.com/juju/1.25-upgrade/juju2/network"
	coretesting "github.com/juju/1.25-upgrade/juju2/testing"
	"github.com/juju/1.25-upgrade/juju2/worker"
	"github.com/juju/1.25-upgrade/juju2/worker/proxyupdater"
)

type acCreator func() (cmd.Command, AgentConf)

// CheckAgentCommand is a utility function for verifying that common agent
// options are handled by a Command; it returns an instance of that
// command pre-parsed, with any mandatory flags added.
func CheckAgentCommand(c *gc.C, create acCreator, args []string) cmd.Command {
	com, conf := create()
	err := coretesting.InitCommand(com, args)
	dataDir, err := paths.DataDir(series.MustHostSeries())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conf.DataDir(), gc.Equals, dataDir)
	badArgs := append(args, "--data-dir", "")
	com, _ = create()
	err = coretesting.InitCommand(com, badArgs)
	c.Assert(err, gc.ErrorMatches, "--data-dir option must be set")

	args = append(args, "--data-dir", "jd")
	com, conf = create()
	c.Assert(coretesting.InitCommand(com, args), gc.IsNil)
	c.Assert(conf.DataDir(), gc.Equals, "jd")
	return com
}

// ParseAgentCommand is a utility function that inserts the always-required args
// before parsing an agent command and returning the result.
func ParseAgentCommand(ac cmd.Command, args []string) error {
	common := []string{
		"--data-dir", "jd",
	}
	return coretesting.InitCommand(ac, append(common, args...))
}

// AgentSuite is a fixture to be used by agent test suites.
type AgentSuite struct {
	agenttest.AgentSuite
}

func (s *AgentSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)

	s.PatchValue(&cmdutil.EnsureMongoServer, func(mongo.EnsureServerParams) error {
		return nil
	})
}

func (s *AgentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	// Set API host ports so FindTools/Tools API calls succeed.
	hostPorts := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&proxyupdater.NewWorker, func(proxyupdater.Config) (worker.Worker, error) {
		return newDummyWorker(), nil
	})

	// Tests should not try to use internet. Ensure base url is empty.
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "")
}
