// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/1.25-upgrade/juju1/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
