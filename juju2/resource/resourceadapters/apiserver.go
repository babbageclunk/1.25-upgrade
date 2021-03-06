// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/apiserver/common"
	"github.com/juju/1.25-upgrade/juju2/apiserver/common/apihttp"
	"github.com/juju/1.25-upgrade/juju2/apiserver/facade"
	"github.com/juju/1.25-upgrade/juju2/resource"
	internalserver "github.com/juju/1.25-upgrade/juju2/resource/api/private/server"
	"github.com/juju/1.25-upgrade/juju2/resource/api/server"
	corestate "github.com/juju/1.25-upgrade/juju2/state"
)

// NewPublicFacade provides the public API facade for resources. It is
// passed into common.RegisterStandardFacade.
func NewPublicFacade(st *corestate.State, _ facade.Resources, authorizer facade.Authorizer) (*server.Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	rst, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newClient := func() (server.CharmStore, error) {
		return newCharmStoreClient(st)
	}
	facade, err := server.NewFacade(rst, newClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// NewApplicationHandler returns a new HTTP handler for application
// level resource uploads and downloads.
func NewApplicationHandler(args apihttp.NewHandlerArgs) http.Handler {
	return server.NewHTTPHandler(
		func(req *http.Request) (server.DataStore, server.Closer, names.Tag, error) {
			st, releaser, entity, err := args.Connect(req)
			if err != nil {
				return nil, nil, nil, errors.Trace(err)
			}
			closer := func() error {
				releaser()
				return nil
			}
			resources, err := st.Resources()
			if err != nil {
				closer()
				return nil, nil, nil, errors.Trace(err)
			}

			return resources, closer, entity.Tag(), nil
		},
	)
}

// NewDownloadHandler returns a new HTTP handler for the given args.
func NewDownloadHandler(args apihttp.NewHandlerArgs) http.Handler {
	extractor := &httpDownloadRequestExtractor{
		connect: args.Connect,
	}
	deps := internalserver.NewHTTPHandlerDeps(extractor)
	return internalserver.NewHTTPHandler(deps)
}

// stateConnector exposes ways to connect to Juju's state.
type stateConnector interface {
	// Connect connects to state for a unit agent.
}

// httpDownloadRequestExtractor provides the functionality needed to
// handle a resource download HTTP request.
type httpDownloadRequestExtractor struct {
	connect func(*http.Request) (*corestate.State, func(), corestate.Entity, error)
}

// NewResourceOpener returns a new resource.Opener for the given
// HTTP request.
func (ex *httpDownloadRequestExtractor) NewResourceOpener(req *http.Request) (opener resource.Opener, err error) {
	st, releaser, _, err := ex.connect(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	closer := func() error {
		releaser()
		return nil
	}

	defer func() {
		if err != nil {
			closer()
		}
	}()

	unitTagStr := req.URL.Query().Get(":unit")
	unitTag, err := names.ParseUnitTag(unitTagStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit, err := st.Unit(unitTag.Id())
	if err != nil {
		return nil, errors.Annotate(err, "loading unit")
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	opener = &resourceOpener{
		st:     st,
		res:    resources,
		userID: unitTag,
		unit:   unit,
		closer: closer,
	}
	return opener, nil
}
