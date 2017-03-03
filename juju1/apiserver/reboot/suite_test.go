// Copyright 2014 Cloudbase Solutions SRL
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot_test

import (
	stdtesting "testing"

	coretesting "github.com/juju/1.25-upgrade/juju1/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}
