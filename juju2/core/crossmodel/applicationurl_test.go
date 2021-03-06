// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	"fmt"
	"regexp"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/1.25-upgrade/juju2/core/crossmodel"
)

type ApplicationURLSuite struct{}

var _ = gc.Suite(&ApplicationURLSuite{})

var urlTests = []struct {
	s, err string
	exact  string
	url    *crossmodel.ApplicationURL
}{{
	s:   "local:/u/user/applicationname",
	url: &crossmodel.ApplicationURL{"local", "user", "", "applicationname"},
}, {
	s:     "u/user/applicationname",
	url:   &crossmodel.ApplicationURL{"local", "user", "", "applicationname"},
	exact: "local:/u/user/applicationname",
}, {
	s:     "u/user/prod/applicationname",
	url:   &crossmodel.ApplicationURL{"local", "user", "prod", "applicationname"},
	exact: "local:/u/user/prod/applicationname",
}, {
	s:   "local:application",
	err: `application URL has invalid form, missing "/u/<user>": $URL`,
}, {
	s:   "local:user/application",
	err: `application URL has invalid form, missing "/u/<user>": $URL`,
}, {
	s:   "local:/u/user",
	err: `application URL has invalid form, missing application name: $URL`,
}, {
	s:   "application",
	err: `application URL has invalid form, missing "/u/<user>": $URL`,
}, {
	s:   "/user/application",
	err: `application URL has invalid form, missing "/u/<user>": $URL`,
}, {
	s:   "/u/user",
	err: `application URL has invalid form, missing application name: $URL`,
}, {
	s:   "local:/u/user/application.bad",
	err: `application name "application.bad" not valid`,
}, {
	s:   "local:/u/user[bad/application",
	err: `user name "user\[bad" not valid`,
}, {
	s:   ":foo",
	err: `cannot parse application URL: $URL`,
}, {
	s:   "local:/u/fred/prod/foo/db2",
	err: `application URL has invalid form: $URL`,
}}

func (s *ApplicationURLSuite) TestParseURL(c *gc.C) {
	for i, t := range urlTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseApplicationURL(t.s)

		match := t.s
		if t.exact != "" {
			match = t.exact
		}
		if t.url != nil {
			c.Assert(err, gc.IsNil)
			c.Assert(url, gc.DeepEquals, t.url)
			c.Check(url.String(), gc.Equals, match)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Assert(err, gc.ErrorMatches, t.err)
			c.Assert(url, gc.IsNil)
		}
	}
}

func (s *ApplicationURLSuite) TestServiceDirectoryForURL(c *gc.C) {
	dir, err := crossmodel.ApplicationDirectoryForURL("local:/u/me/application")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir, gc.Equals, "local")
}

func (s *ApplicationURLSuite) TestServiceDirectoryForURLError(c *gc.C) {
	_, err := crossmodel.ApplicationDirectoryForURL("error")
	c.Assert(err, gc.ErrorMatches, "application URL has invalid form.*")
}

var urlPartsTests = []struct {
	s, err string
	url    *crossmodel.ApplicationURLParts
}{{
	s:   "local:/u/user/applicationname",
	url: &crossmodel.ApplicationURLParts{"local", "user", "", "applicationname"},
}, {
	s:   "u/user/applicationname",
	url: &crossmodel.ApplicationURLParts{"", "user", "", "applicationname"},
}, {
	s:   "u/user/prod/applicationname",
	url: &crossmodel.ApplicationURLParts{"", "user", "prod", "applicationname"},
}, {
	s:   "u/user",
	url: &crossmodel.ApplicationURLParts{"", "user", "", ""},
}, {
	s:   "application",
	url: &crossmodel.ApplicationURLParts{"", "", "", "application"},
}, {
	s:   "prod/application",
	url: &crossmodel.ApplicationURLParts{"", "", "prod", "application"},
}, {
	s:   "local:/application",
	url: &crossmodel.ApplicationURLParts{"local", "", "", "application"},
}, {
	s:   "",
	url: &crossmodel.ApplicationURLParts{},
}, {
	s:   "a/b/c",
	err: `application URL has too many parts: "a/b/c"`,
}, {
	s:   "local:/u/user/application.bad",
	err: `application name "application.bad" not valid`,
}, {
	s:   "local:/u/user[bad/application",
	err: `user name "user\[bad" not valid`,
}, {
	s:   ":foo",
	err: `cannot parse application URL: $URL`,
}}

func (s *ApplicationURLSuite) TestParseURLParts(c *gc.C) {
	for i, t := range urlPartsTests {
		c.Logf("test %d: %q", i, t.s)
		url, err := crossmodel.ParseApplicationURLParts(t.s)

		if t.url != nil {
			c.Check(err, gc.IsNil)
			c.Check(url, gc.DeepEquals, t.url)
		}
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Assert(err, gc.ErrorMatches, t.err)
			c.Assert(url, gc.IsNil)
		}
	}
}
