// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/1.25-upgrade/juju2/cloudconfig/instancecfg"
	"github.com/juju/1.25-upgrade/juju2/constraints"
	"github.com/juju/1.25-upgrade/juju2/environs/config"
	"github.com/juju/1.25-upgrade/juju2/environs/imagemetadata"
	"github.com/juju/1.25-upgrade/juju2/instance"
	"github.com/juju/1.25-upgrade/juju2/network"
	"github.com/juju/1.25-upgrade/juju2/status"
	"github.com/juju/1.25-upgrade/juju2/storage"
	"github.com/juju/1.25-upgrade/juju2/tools"
)

// StartInstanceParams holds parameters for the
// InstanceBroker.StartInstance method.
type StartInstanceParams struct {
	// ControllerUUID is the uuid of the controller.
	ControllerUUID string

	// Constraints is a set of constraints on
	// the kind of instance to create.
	Constraints constraints.Value

	// Tools is a list of tools that may be used
	// to start a Juju agent on the machine.
	Tools tools.List

	// InstanceConfig describes the machine's configuration.
	InstanceConfig *instancecfg.InstanceConfig

	// Placement, if non-empty, contains an environment-specific
	// placement directive that may be used to decide how the
	// instance should be started.
	Placement string

	// DistributionGroup, if non-nil, is a function
	// that returns a slice of instance.Ids that belong
	// to the same distribution group as the machine
	// being provisioned. The InstanceBroker may use
	// this information to distribute instances for
	// high availability.
	DistributionGroup func() ([]instance.Id, error)

	// Volumes is a set of parameters for volumes that should be created.
	//
	// StartInstance need not check the value of the Attachment field,
	// as it is guaranteed that any volumes in this list are designated
	// for attachment to the instance being started.
	Volumes []storage.VolumeParams

	// NetworkInfo is an optional list of network interface details,
	// necessary to configure on the instance.
	NetworkInfo []network.InterfaceInfo

	// SubnetsToZones is an optional map of provider-specific subnet
	// id to a list of availability zone names the subnet is available
	// in. It is only populated when valid positive spaces constraints
	// are present.
	SubnetsToZones map[network.Id][]string

	// EndpointBindings holds the mapping between service endpoint names to
	// provider-specific space IDs. It is populated when provisioning a machine
	// to host a unit of a service with endpoint bindings.
	EndpointBindings map[string]network.Id
	// ImageMetadata is a collection of image metadata
	// that may be used to start this instance.
	ImageMetadata []*imagemetadata.ImageMetadata

	// StatusCallback is a callback to be used by the instance to report
	// changes in status. Its signature is consistent with other
	// status-related functions to allow them to be used as callbacks.
	StatusCallback func(settableStatus status.Status, info string, data map[string]interface{}) error

	// CleanupCallback is a callback to be used to clean up any residual
	// status-reporting output from StatusCallback.
	CleanupCallback func(info string) error
}

// StartInstanceResult holds the result of an
// InstanceBroker.StartInstance method call.
type StartInstanceResult struct {
	// Instance is an interface representing a cloud instance.
	Instance instance.Instance

	// Config holds the environment config to be used for any further
	// operations, if the instance is for a controller.
	Config *config.Config

	// HardwareCharacteristics represents the hardware characteristics
	// of the newly created instance.
	Hardware *instance.HardwareCharacteristics

	// NetworkInfo contains information about how to configure network
	// interfaces on the instance. Depending on the provider, this
	// might be the same StartInstanceParams.NetworkInfo or may be
	// modified as needed.
	NetworkInfo []network.InterfaceInfo

	// Volumes contains a list of volumes created, each one having the
	// same Name as one of the VolumeParams in StartInstanceParams.Volumes.
	// VolumeAttachment information is reported separately.
	Volumes []storage.Volume

	// VolumeAttachments contains a attachment-specific information about
	// volumes that were attached to the started instance.
	VolumeAttachments []storage.VolumeAttachment
}

// TODO(wallyworld) - we want this in the environs/instance package but import loops
// stop that from being possible right now.
type InstanceBroker interface {
	// StartInstance asks for a new instance to be created, associated with
	// the provided config in machineConfig. The given config describes the juju
	// state for the new instance to connect to. The config MachineNonce, which must be
	// unique within an environment, is used by juju to protect against the
	// consequences of multiple instances being started with the same machine
	// id.
	StartInstance(args StartInstanceParams) (*StartInstanceResult, error)

	// StopInstances shuts down the instances with the specified IDs.
	// Unknown instance IDs are ignored, to enable idempotency.
	StopInstances(...instance.Id) error

	// AllInstances returns all instances currently known to the broker.
	AllInstances() ([]instance.Instance, error)

	// MaintainInstance is used to run actions on jujud startup for existing
	// instances. It is currently only used to ensure that LXC hosts have the
	// correct network configuration.
	MaintainInstance(args StartInstanceParams) error
}
