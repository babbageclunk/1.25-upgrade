// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

// CharmResolver interface defines the functionality to resolve a charm URL.
type CharmResolver interface {
	// Resolve resolves the charm URL.
	Resolve(client *httpbakery.Client, charmURL string) (string, error)
}

// CharmStoreResolver implements the CharmResolver interface.
type CharmStoreResolver struct {
	csURL string
}

// NewCharmStoreResolver creates a new charm store resolver.
func NewCharmStoreResolver() *CharmStoreResolver {
	return &CharmStoreResolver{
		csURL: csclient.ServerURL,
	}
}

// Resolve implements the CharmResolver interface.
func (r *CharmStoreResolver) Resolve(client *httpbakery.Client, charmURL string) (string, error) {
	repo := charmrepo.NewCharmStore(charmrepo.NewCharmStoreParams{
		BakeryClient: client,
		URL:          r.csURL,
	})

	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return "", errors.Annotate(err, "could not parse charm url")
	}
	// ignore local charm urls
	if curl.Schema == "local" {
		return charmURL, nil
	}
	resolvedURL, _, err := repo.Resolve(curl)
	if err != nil {
		return "", errors.Trace(err)
	}
	return resolvedURL.String(), nil
}
