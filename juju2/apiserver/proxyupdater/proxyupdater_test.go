// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/apiserver/common"
	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	"github.com/juju/1.25-upgrade/juju2/apiserver/proxyupdater"
	apiservertesting "github.com/juju/1.25-upgrade/juju2/apiserver/testing"
	"github.com/juju/1.25-upgrade/juju2/environs/config"
	"github.com/juju/1.25-upgrade/juju2/network"
	"github.com/juju/1.25-upgrade/juju2/state"
	coretesting "github.com/juju/1.25-upgrade/juju2/testing"
	"github.com/juju/1.25-upgrade/juju2/worker/workertest"
	"github.com/juju/testing"
)

type ProxyUpdaterSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	state      *stubBackend
	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *proxyupdater.ProxyUpdaterAPI
	tag        names.MachineTag
}

var _ = gc.Suite(&ProxyUpdaterSuite{})

func (s *ProxyUpdaterSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.StubNetwork.SetUpSuite(c)
}

func (s *ProxyUpdaterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	s.tag = names.NewMachineTag("1")
	s.state = &stubBackend{}
	s.state.SetUp(c)
	s.AddCleanup(func(_ *gc.C) { s.state.Kill() })

	var err error
	s.facade, err = proxyupdater.NewAPIWithBacking(s.state, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)

	// Shouldn't have any calls yet
	apiservertesting.CheckMethodCalls(c, s.state.Stub)
}

func (s *ProxyUpdaterSuite) TestWatchForProxyConfigAndAPIHostPortChanges(c *gc.C) {
	// WatchForProxyConfigAndAPIHostPortChanges combines WatchForModelConfigChanges
	// and WatchAPIHostPorts. Check that they are both called and we get the
	result := s.facade.WatchForProxyConfigAndAPIHostPortChanges(s.oneEntity())
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)

	s.state.Stub.CheckCallNames(c,
		"WatchForModelConfigChanges",
		"WatchAPIHostPorts",
	)

	// Verify the watcher resource was registered.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get(result.Results[0].NotifyWatcherId)
	watcher, ok := resource.(state.NotifyWatcher)
	c.Assert(ok, jc.IsTrue)

	// Verify the initial event was consumed.
	select {
	case <-watcher.Changes():
		c.Fatalf("initial event never consumed")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *ProxyUpdaterSuite) oneEntity() params.Entities {
	entities := params.Entities{
		make([]params.Entity, 1),
	}
	entities.Entities[0].Tag = s.tag.String()
	return entities
}

func (s *ProxyUpdaterSuite) TestProxyConfig(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	cfg := s.facade.ProxyConfig(s.oneEntity())

	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPorts",
	)

	noProxy := "0.1.2.3,0.1.2.4,0.1.2.5"

	r := params.ProxyConfigResult{
		ProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: noProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://http proxy", HTTPS: "https://https proxy", FTP: "", NoProxy: ""},
	}
	c.Assert(cfg.Results[0], jc.DeepEquals, r)
}

func (s *ProxyUpdaterSuite) TestProxyConfigExtendsExisting(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	s.state.SetModelConfig(coretesting.Attrs{
		"http-proxy":  "http proxy",
		"https-proxy": "https proxy",
		"no-proxy":    "9.9.9.9",
	})
	cfg := s.facade.ProxyConfig(s.oneEntity())
	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPorts",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5,9.9.9.9"

	c.Assert(cfg.Results[0], jc.DeepEquals, params.ProxyConfigResult{
		ProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://http proxy", HTTPS: "https://https proxy", FTP: "", NoProxy: ""},
	})
}

func (s *ProxyUpdaterSuite) TestProxyConfigNoDuplicates(c *gc.C) {
	// Check that the ProxyConfig combines data from ModelConfig and APIHostPorts
	s.state.SetModelConfig(coretesting.Attrs{
		"http-proxy":  "http proxy",
		"https-proxy": "https proxy",
		"no-proxy":    "0.1.2.3",
	})
	cfg := s.facade.ProxyConfig(s.oneEntity())
	s.state.Stub.CheckCallNames(c,
		"ModelConfig",
		"APIHostPorts",
	)

	expectedNoProxy := "0.1.2.3,0.1.2.4,0.1.2.5"

	c.Assert(cfg.Results[0], jc.DeepEquals, params.ProxyConfigResult{
		ProxySettings: params.ProxyConfig{
			HTTP: "http proxy", HTTPS: "https proxy", FTP: "", NoProxy: expectedNoProxy},
		APTProxySettings: params.ProxyConfig{
			HTTP: "http://http proxy", HTTPS: "https://https proxy", FTP: "", NoProxy: ""},
	})
}

type stubBackend struct {
	*testing.Stub

	EnvConfig   *config.Config
	c           *gc.C
	configAttrs coretesting.Attrs
	hpWatcher   workertest.NotAWatcher
	confWatcher workertest.NotAWatcher
}

func (sb *stubBackend) SetUp(c *gc.C) {
	sb.Stub = &testing.Stub{}
	sb.c = c
	sb.configAttrs = coretesting.Attrs{
		"http-proxy":  "http proxy",
		"https-proxy": "https proxy",
	}
	sb.hpWatcher = workertest.NewFakeWatcher(1, 1)
	sb.confWatcher = workertest.NewFakeWatcher(1, 1)
}

func (sb *stubBackend) Kill() {
	sb.hpWatcher.Kill()
	sb.confWatcher.Kill()
}

func (sb *stubBackend) SetModelConfig(ca coretesting.Attrs) {
	sb.configAttrs = ca
}

func (sb *stubBackend) ModelConfig() (*config.Config, error) {
	sb.MethodCall(sb, "ModelConfig")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	return coretesting.CustomModelConfig(sb.c, sb.configAttrs), nil
}

func (sb *stubBackend) APIHostPorts() ([][]network.HostPort, error) {
	sb.MethodCall(sb, "APIHostPorts")
	if err := sb.NextErr(); err != nil {
		return nil, err
	}
	hps := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
		network.NewHostPorts(1234, "0.1.2.4"),
		network.NewHostPorts(1234, "0.1.2.5"),
	}
	return hps, nil
}

func (sb *stubBackend) WatchAPIHostPorts() state.NotifyWatcher {
	sb.MethodCall(sb, "WatchAPIHostPorts")
	return sb.hpWatcher
}

func (sb *stubBackend) WatchForModelConfigChanges() state.NotifyWatcher {
	sb.MethodCall(sb, "WatchForModelConfigChanges")
	return sb.confWatcher
}
