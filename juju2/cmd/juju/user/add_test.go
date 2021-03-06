// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package user_test

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/api/base"
	"github.com/juju/1.25-upgrade/juju2/apiserver/common"
	"github.com/juju/1.25-upgrade/juju2/apiserver/params"
	"github.com/juju/1.25-upgrade/juju2/cmd/juju/user"
	"github.com/juju/1.25-upgrade/juju2/testing"
)

// All of the functionality of the AddUser api call is contained elsewhere.
// This suite provides basic tests for the "add-user" command
type UserAddCommandSuite struct {
	BaseSuite
	mockAPI *mockAddUserAPI
}

var _ = gc.Suite(&UserAddCommandSuite{})

func (s *UserAddCommandSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.mockAPI = &mockAddUserAPI{}
	s.mockAPI.secretKey = []byte(strings.Repeat("X", 32))
}

func (s *UserAddCommandSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	addCommand, _ := user.NewAddCommandForTest(s.mockAPI, s.store, &mockModelAPI{})
	return testing.RunCommand(c, addCommand, args...)
}

func (s *UserAddCommandSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		user        string
		displayname string
		models      string
		acl         string
		outPath     string
		errorString string
	}{{
		errorString: "no username supplied",
	}, {
		args: []string{"foobar"},
		user: "foobar",
	}, {
		args:        []string{"foobar", "Foo Bar"},
		user:        "foobar",
		displayname: "Foo Bar",
	}, {
		args:        []string{"foobar", "Foo Bar", "extra"},
		errorString: `unrecognized args: \["extra"\]`,
	}, {
		args: []string{"foobar"},
		user: "foobar",
	}, {
		args: []string{"foobar"},
		user: "foobar",
	}} {
		c.Logf("test %d (%q)", i, test.args)
		wrappedCommand, command := user.NewAddCommandForTest(s.mockAPI, s.store, &mockModelAPI{})
		err := testing.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(command.User, gc.Equals, test.user)
			c.Check(command.DisplayName, gc.Equals, test.displayname)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}

func (s *UserAddCommandSuite) TestAddUserWithUsername(c *gc.C) {
	context, err := s.run(c, "foobar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "")
	expected := `
User "foobar" added
Please send this command to foobar:
    juju register MEYTBmZvb2JhcjAREw8xMjcuMC4wLjE6MTIzNDUEIFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYEwd0ZXN0aW5n

"foobar" has not been granted access to any models. You can use "juju grant" to grant access.
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

func (s *UserAddCommandSuite) TestAddUserWithUsernameAndDisplayname(c *gc.C) {
	context, err := s.run(c, "foobar", "Foo Bar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mockAPI.username, gc.Equals, "foobar")
	c.Assert(s.mockAPI.displayname, gc.Equals, "Foo Bar")
	expected := `
User "Foo Bar (foobar)" added
Please send this command to foobar:
    juju register MEYTBmZvb2JhcjAREw8xMjcuMC4wLjE6MTIzNDUEIFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYEwd0ZXN0aW5n

"Foo Bar (foobar)" has not been granted access to any models. You can use "juju grant" to grant access.
`[1:]
	c.Assert(testing.Stdout(context), gc.Equals, expected)
	c.Assert(testing.Stderr(context), gc.Equals, "")
}

type mockModelAPI struct{}

func (m *mockModelAPI) ListModels(user string) ([]base.UserModel, error) {
	return []base.UserModel{{Name: "model", UUID: "modeluuid", Owner: "current-user"}}, nil
}

func (m *mockModelAPI) Close() error {
	return nil
}

func (s *UserAddCommandSuite) TestBlockAddUser(c *gc.C) {
	// Block operation
	s.mockAPI.blocked = true
	_, err := s.run(c, "foobar", "Foo Bar")
	testing.AssertOperationWasBlocked(c, err, ".*To enable changes.*")
}

func (s *UserAddCommandSuite) TestAddUserErrorResponse(c *gc.C) {
	s.mockAPI.failMessage = "failed to create user, chaos ensues"
	_, err := s.run(c, "foobar")
	c.Assert(err, gc.ErrorMatches, s.mockAPI.failMessage)
}

func (s *UserAddCommandSuite) TestAddUserUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.mockAPI.addError = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := s.run(c, "foobar")
	errString := strings.Replace(testing.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
}

type mockAddUserAPI struct {
	addError    error
	failMessage string
	blocked     bool
	secretKey   []byte

	username    string
	displayname string
	password    string
}

func (m *mockAddUserAPI) AddUser(username, displayname, password string) (names.UserTag, []byte, error) {
	if m.blocked {
		return names.UserTag{}, nil, common.OperationBlockedError("the operation has been blocked")
	}
	if m.addError != nil {
		return names.UserTag{}, nil, m.addError
	}
	m.username = username
	m.displayname = displayname
	m.password = password
	if m.failMessage != "" {
		return names.UserTag{}, nil, errors.New(m.failMessage)
	}
	return names.NewLocalUserTag(username), m.secretKey, nil
}

func (*mockAddUserAPI) Close() error {
	return nil
}
