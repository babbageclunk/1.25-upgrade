// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package upgrades

import (
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/1.25-upgrade/juju2/provider/lxd"
	"github.com/juju/1.25-upgrade/juju2/state"
)

func updateLXDCloudCredentials(st *state.State) error {
	creds, err := lxd.ReadLegacyCloudCredentials(ioutil.ReadFile)
	if err != nil {
		if errors.IsNotFound(err) {
			// Not running a LXD controller.
			return nil
		}
		return errors.Annotate(err, "reading credentials from disk")
	}
	gatewayAddress, err := getDefaultGateway()
	if err != nil {
		return errors.Annotate(err, "reading gateway address")
	}
	return state.UpdateLegacyLXDCloudCredentials(st, gatewayAddress, creds)
}

func getDefaultGateway() (string, error) {
	out, err := utils.RunCommand("ip", "route", "list", "match", "0/0")
	if err != nil {
		return "", errors.Trace(err)
	}
	if !strings.HasPrefix(string(out), "default via") {
		return "", errors.Errorf(`unexpected output from "ip route": %s`, out)
	}
	fields := strings.Fields(string(out))
	return fields[2], nil
}
