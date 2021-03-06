// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/1.25-upgrade/juju2/apiserver/common"
	"github.com/juju/1.25-upgrade/juju2/cmd/juju/model"
	"github.com/juju/1.25-upgrade/juju2/testing"
)

type ConfigCommandSuite struct {
	fakeEnvSuite
}

var _ = gc.Suite(&ConfigCommandSuite{})

func (s *ConfigCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	command := model.NewConfigCommandForTest(s.fake)
	return testing.RunCommand(c, command, args...)
}

func (s *ConfigCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		desc       string
		args       []string
		errorMatch string
		nilErr     bool
	}{
		{ // Test set
			desc:       "keys cannot be duplicates",
			args:       []string{"special=extra", "special=other"},
			errorMatch: `key "special" specified more than once`,
		}, {
			desc:       "agent-version cannot be set",
			args:       []string{"agent-version=2.0.0"},
			errorMatch: `agent-version must be set via "upgrade-juju"`,
		}, {
			// Test reset
			desc:       "reset requires arg",
			args:       []string{"--reset"},
			errorMatch: "flag needs an argument: --reset",
		}, {
			desc:       "cannot set and retrieve at the same time",
			args:       []string{"--reset", "something", "weird"},
			errorMatch: "cannot set and retrieve model values simultaneously",
		}, {
			desc:       "agent-version cannot be reset",
			args:       []string{"--reset", "agent-version"},
			errorMatch: `"agent-version" cannot be reset`,
		}, {
			desc:       "set and reset cannot have duplicate keys",
			args:       []string{"--reset", "special", "special=extra"},
			errorMatch: `key "special" cannot be both set and reset in the same command`,
		}, {
			desc:       "reset cannot have k=v pairs",
			args:       []string{"--reset", "a,b,c=d,e"},
			errorMatch: `--reset accepts a comma delimited set of keys "a,b,c", received: "c=d"`,
		}, {
			// Test get
			desc:   "get all succeds",
			args:   nil,
			nilErr: true,
		}, {
			desc:   "get one succeeds",
			args:   []string{"one"},
			nilErr: true,
		}, {
			desc:       "get multiple fails",
			args:       []string{"one", "two"},
			errorMatch: "can only retrieve a single value, or all values",
		}, {
			// test variations
			desc:   "test reset interspersed",
			args:   []string{"--reset", "one", "special=foo", "--reset", "two"},
			nilErr: true,
		},
	} {
		c.Logf("test %d: %s", i, test.desc)
		cmd := model.NewConfigCommandForTest(s.fake)
		err := testing.InitCommand(cmd, test.args)
		if test.nilErr {
			c.Check(err, jc.ErrorIsNil)
			continue
		}
		c.Check(err, gc.ErrorMatches, test.errorMatch)
	}
}

func (s *ConfigCommandSuite) TestSingleValue(c *gc.C) {
	s.fake.values["special"] = "multi\nline"

	context, err := s.run(c, "special")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, "multi\nline\n")
}

func (s *ConfigCommandSuite) TestSingleValueOutputFile(c *gc.C) {
	s.fake.values["special"] = "multi\nline"

	outpath := filepath.Join(c.MkDir(), "out")
	_, err := s.run(c, "--output", outpath, "special")
	c.Assert(err, jc.ErrorIsNil)

	output, err := ioutil.ReadFile(outpath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Equals, "multi\nline\n")
}

func (s *ConfigCommandSuite) TestGetUnknownValue(c *gc.C) {
	context, err := s.run(c, "unknown")
	c.Assert(err, gc.ErrorMatches, `key "unknown" not found in {<nil> ""} model.`)

	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, "")
}

func (s *ConfigCommandSuite) TestSingleValueJSON(c *gc.C) {
	context, err := s.run(c, "--format=json", "special")
	c.Assert(err, jc.ErrorIsNil)

	want := "{\"special\":{\"Value\":\"special value\",\"Source\":\"model\"}}\n"
	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *ConfigCommandSuite) TestSingleValueYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml", "special")
	c.Assert(err, jc.ErrorIsNil)

	want := "" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"

	output := testing.Stdout(context)
	c.Assert(output, gc.Equals, want)
}

func (s *ConfigCommandSuite) TestAllValuesYAML(c *gc.C) {
	context, err := s.run(c, "--format=yaml")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	expected := "" +
		"running:\n" +
		"  value: true\n" +
		"  source: model\n" +
		"special:\n" +
		"  value: special value\n" +
		"  source: model\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesJSON(c *gc.C) {
	context, err := s.run(c, "--format=json")
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	expected := `{"running":{"Value":true,"Source":"model"},"special":{"Value":"special value","Source":"model"}}` + "\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestAllValuesTabular(c *gc.C) {
	context, err := s.run(c)
	c.Assert(err, jc.ErrorIsNil)

	output := testing.Stdout(context)
	expected := "" +
		"Attribute  From   Value\n" +
		"running    model  true\n" +
		"special    model  special value\n" +
		"\n"
	c.Assert(output, gc.Equals, expected)
}

func (s *ConfigCommandSuite) TestPassesValues(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	expected := map[string]interface{}{
		"special": "extra",
		"unknown": "foo",
	}
	c.Assert(s.fake.values, jc.DeepEquals, expected)
}

func (s *ConfigCommandSuite) TestSettingKnownValue(c *gc.C) {
	_, err := s.run(c, "special=extra", "unknown=foo")
	c.Assert(err, jc.ErrorIsNil)
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *ConfigCommandSuite) TestBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "special=extra")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}

func (s *ConfigCommandSuite) TestResetPassesValues(c *gc.C) {
	_, err := s.run(c, "--reset", "special,running")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.resetKeys, jc.DeepEquals, []string{"special", "running"})
}

func (s *ConfigCommandSuite) TestResettingUnKnownValue(c *gc.C) {
	_, err := s.run(c, "--reset", "unknown")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.resetKeys, jc.DeepEquals, []string{"unknown"})
	// Command succeeds, but warning logged.
	expected := `key "unknown" is not defined in the current model configuration: possible misspelling`
	c.Check(c.GetTestLog(), jc.Contains, expected)
}

func (s *ConfigCommandSuite) TestResetBlockedError(c *gc.C) {
	s.fake.err = common.OperationBlockedError("TestBlockedError")
	_, err := s.run(c, "--reset", "special")
	testing.AssertOperationWasBlocked(c, err, ".*TestBlockedError.*")
}
