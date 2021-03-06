// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"gopkg.in/juju/charm.v6-unstable"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/state"
)

type charmsAccess interface {
	Charm(curl *charm.URL) (*state.Charm, error)
	AllCharms() ([]*state.Charm, error)
	ModelTag() names.ModelTag
}

type stateShim struct {
	state *state.State
}

func (s stateShim) Charm(curl *charm.URL) (*state.Charm, error) {
	return s.state.Charm(curl)
}

func (s stateShim) AllCharms() ([]*state.Charm, error) {
	return s.state.AllCharms()
}

func (s stateShim) ModelTag() names.ModelTag {
	return s.state.ModelTag()
}
