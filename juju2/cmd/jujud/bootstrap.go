// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/ssh"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"

	"github.com/juju/1.25-upgrade/juju2/agent"
	"github.com/juju/1.25-upgrade/juju2/agent/agentbootstrap"
	agenttools "github.com/juju/1.25-upgrade/juju2/agent/tools"
	"github.com/juju/1.25-upgrade/juju2/cloudconfig/instancecfg"
	agentcmd "github.com/juju/1.25-upgrade/juju2/cmd/jujud/agent"
	cmdutil "github.com/juju/1.25-upgrade/juju2/cmd/jujud/util"
	"github.com/juju/1.25-upgrade/juju2/environs"
	"github.com/juju/1.25-upgrade/juju2/environs/config"
	"github.com/juju/1.25-upgrade/juju2/environs/imagemetadata"
	"github.com/juju/1.25-upgrade/juju2/environs/simplestreams"
	envtools "github.com/juju/1.25-upgrade/juju2/environs/tools"
	"github.com/juju/1.25-upgrade/juju2/instance"
	"github.com/juju/1.25-upgrade/juju2/mongo"
	"github.com/juju/1.25-upgrade/juju2/network"
	"github.com/juju/1.25-upgrade/juju2/state"
	"github.com/juju/1.25-upgrade/juju2/state/binarystorage"
	"github.com/juju/1.25-upgrade/juju2/state/cloudimagemetadata"
	"github.com/juju/1.25-upgrade/juju2/state/multiwatcher"
	"github.com/juju/1.25-upgrade/juju2/state/stateenvirons"
	"github.com/juju/1.25-upgrade/juju2/tools"
	jujuversion "github.com/juju/1.25-upgrade/juju2/version"
	"github.com/juju/1.25-upgrade/juju2/worker/peergrouper"
)

var (
	initiateMongoServer  = peergrouper.InitiateMongoServer
	agentInitializeState = agentbootstrap.InitializeState
	sshGenerateKey       = ssh.GenerateKey
	minSocketTimeout     = 1 * time.Minute
	logger               = loggo.GetLogger("juju.cmd.jujud")
)

const adminUserName = "admin"

// BootstrapCommand represents a jujud bootstrap command.
type BootstrapCommand struct {
	cmd.CommandBase
	agentcmd.AgentConf
	BootstrapParamsFile string
	Timeout             time.Duration
}

// NewBootstrapCommand returns a new BootstrapCommand that has been initialized.
func NewBootstrapCommand() *BootstrapCommand {
	return &BootstrapCommand{
		AgentConf: agentcmd.NewAgentConf(""),
	}
}

// Info returns a decription of the command.
func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap-state",
		Purpose: "initialize juju state",
	}
}

// SetFlags adds the flags for this command to the passed gnuflag.FlagSet.
func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.AgentConf.AddFlags(f)
	f.DurationVar(&c.Timeout, "timeout", time.Duration(0), "set the bootstrap timeout")
}

// Init initializes the command for running.
func (c *BootstrapCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("bootstrap-params file must be specified")
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	c.BootstrapParamsFile = args[0]
	return c.AgentConf.CheckArgs(args[1:])
}

// Run initializes state for an environment.
func (c *BootstrapCommand) Run(_ *cmd.Context) error {
	bootstrapParamsData, err := ioutil.ReadFile(c.BootstrapParamsFile)
	if err != nil {
		return errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return errors.Trace(err)
	}

	err = c.ReadConfig("machine-0")
	if err != nil {
		return errors.Annotate(err, "cannot read config")
	}
	agentConfig := c.CurrentConfig()

	// agent.Jobs is an optional field in the agent config, and was
	// introduced after 1.17.2. We default to allowing units on
	// machine-0 if missing.
	jobs := agentConfig.Jobs()
	if len(jobs) == 0 {
		jobs = []multiwatcher.MachineJob{
			multiwatcher.JobManageModel,
			multiwatcher.JobHostUnits,
		}
	}

	// Get the bootstrap machine's addresses from the provider.
	cloudSpec, err := environs.MakeCloudSpec(
		args.ControllerCloud,
		args.ControllerCloudRegion,
		args.ControllerCloudCredential,
	)
	if err != nil {
		return errors.Trace(err)
	}
	env, err := environs.New(environs.OpenParams{
		Cloud:  cloudSpec,
		Config: args.ControllerModelConfig,
	})
	if err != nil {
		return errors.Annotate(err, "new environ")
	}
	newConfigAttrs := make(map[string]interface{})

	// Check to see if a newer agent version has been requested
	// by the bootstrap client.
	desiredVersion, ok := args.ControllerModelConfig.AgentVersion()
	if ok && desiredVersion != jujuversion.Current {
		// If we have been asked for a newer version, ensure the newer
		// tools can actually be found, or else bootstrap won't complete.
		stream := envtools.PreferredStream(&desiredVersion, args.ControllerModelConfig.Development(), args.ControllerModelConfig.AgentStream())
		logger.Infof("newer tools requested, looking for %v in stream %v", desiredVersion, stream)
		hostSeries, err := series.HostSeries()
		if err != nil {
			return errors.Trace(err)
		}
		filter := tools.Filter{
			Number: desiredVersion,
			Arch:   arch.HostArch(),
			Series: hostSeries,
		}
		_, toolsErr := envtools.FindTools(env, -1, -1, stream, filter)
		if toolsErr == nil {
			logger.Infof("tools are available, upgrade will occur after bootstrap")
		}
		if errors.IsNotFound(toolsErr) {
			// Newer tools not available, so revert to using the tools
			// matching the current agent version.
			logger.Warningf("newer tools for %q not available, sticking with version %q", desiredVersion, jujuversion.Current)
			newConfigAttrs["agent-version"] = jujuversion.Current.String()
		} else if toolsErr != nil {
			logger.Errorf("cannot find newer tools: %v", toolsErr)
			return toolsErr
		}
	}

	instances, err := env.Instances([]instance.Id{args.BootstrapMachineInstanceId})
	if err != nil {
		return errors.Annotate(err, "getting bootstrap instance")
	}
	addrs, err := instances[0].Addresses()
	if err != nil {
		return errors.Annotate(err, "bootstrap instance addresses")
	}

	// When machine addresses are reported from state, they have
	// duplicates removed.  We should do the same here so that
	// there is not unnecessary churn in the mongo replicaset.
	// TODO (cherylj) Add explicit unit tests for this - tracked
	// by bug #1544158.
	addrs = network.MergedAddresses([]network.Address{}, addrs)

	// Generate a private SSH key for the controllers, and add
	// the public key to the environment config. We'll add the
	// private key to StateServingInfo below.
	privateKey, publicKey, err := sshGenerateKey(config.JujuSystemKey)
	if err != nil {
		return errors.Annotate(err, "failed to generate system key")
	}
	authorizedKeys := config.ConcatAuthKeys(args.ControllerModelConfig.AuthorizedKeys(), publicKey)
	newConfigAttrs[config.AuthorizedKeysKey] = authorizedKeys

	// Generate a shared secret for the Mongo replica set, and write it out.
	sharedSecret, err := mongo.GenerateSharedSecret()
	if err != nil {
		return err
	}
	info, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("bootstrap machine config has no state serving info")
	}
	info.SharedSecret = sharedSecret
	info.SystemIdentity = privateKey
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		agentConfig.SetStateServingInfo(info)
		return nil
	})
	if err != nil {
		return fmt.Errorf("cannot write agent config: %v", err)
	}

	agentConfig = c.CurrentConfig()

	// Create system-identity file
	if err := agent.WriteSystemIdentityFile(agentConfig); err != nil {
		return err
	}

	if err := c.startMongo(addrs, agentConfig); err != nil {
		return errors.Annotate(err, "failed to start mongo")
	}

	controllerModelCfg, err := env.Config().Apply(newConfigAttrs)
	if err != nil {
		return errors.Annotate(err, "failed to update model config")
	}
	args.ControllerModelConfig = controllerModelCfg

	// Initialise state, and store any agent config (e.g. password) changes.
	var st *state.State
	var m *state.Machine
	err = c.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		var stateErr error
		dialOpts := mongo.DefaultDialOpts()

		// Set a longer socket timeout than usual, as the machine
		// will be starting up and disk I/O slower than usual. This
		// has been known to cause timeouts in queries.
		dialOpts.SocketTimeout = c.Timeout
		if dialOpts.SocketTimeout < minSocketTimeout {
			dialOpts.SocketTimeout = minSocketTimeout
		}

		// We shouldn't attempt to dial peers until we have some.
		dialOpts.Direct = true

		adminTag := names.NewLocalUserTag(adminUserName)
		st, m, stateErr = agentInitializeState(
			adminTag,
			agentConfig,
			agentbootstrap.InitializeStateParams{
				StateInitializationParams: args,
				BootstrapMachineAddresses: addrs,
				BootstrapMachineJobs:      jobs,
				SharedSecret:              sharedSecret,
				Provider:                  environs.Provider,
				StorageProviderRegistry:   stateenvirons.NewStorageProviderRegistry(env),
			},
			dialOpts,
			stateenvirons.GetNewPolicyFunc(
				stateenvirons.GetNewEnvironFunc(environs.New),
			),
		)
		return stateErr
	})
	if err != nil {
		return err
	}
	defer st.Close()

	// Populate the tools catalogue.
	if err := c.populateTools(st, env); err != nil {
		return err
	}

	// Populate the GUI archive catalogue.
	if err := c.populateGUIArchive(st, env); err != nil {
		// Do not stop the bootstrapping process for Juju GUI archive errors.
		logger.Warningf("cannot set up Juju GUI: %s", err)
	} else {
		logger.Debugf("Juju GUI successfully set up")
	}

	// Add custom image metadata to environment storage.
	if len(args.CustomImageMetadata) > 0 {
		if err := c.saveCustomImageMetadata(st, env, args.CustomImageMetadata); err != nil {
			return err
		}
	}

	// bootstrap machine always gets the vote
	return m.SetHasVote(true)
}

func (c *BootstrapCommand) startMongo(addrs []network.Address, agentConfig agent.Config) error {
	logger.Debugf("starting mongo")

	info, ok := agentConfig.MongoInfo()
	if !ok {
		return fmt.Errorf("no state info available")
	}
	// When bootstrapping, we need to allow enough time for mongo
	// to start as there's no retry loop in place.
	// 5 minutes should suffice.
	mongoDialOpts := mongo.DialOpts{Timeout: 5 * time.Minute}
	dialInfo, err := mongo.DialInfo(info.Info, mongoDialOpts)
	if err != nil {
		return err
	}
	servingInfo, ok := agentConfig.StateServingInfo()
	if !ok {
		return fmt.Errorf("agent config has no state serving info")
	}
	// Use localhost to dial the mongo server, because it's running in
	// auth mode and will refuse to perform any operations unless
	// we dial that address.
	// TODO(macgreagoir) IPv6. Ubuntu still always provides IPv4 loopback,
	// and when/if this changes localhost should resolve to IPv6 loopback
	// in any case (lp:1644009). Review.
	dialInfo.Addrs = []string{
		net.JoinHostPort("localhost", fmt.Sprint(servingInfo.StatePort)),
	}

	logger.Debugf("calling ensureMongoServer")
	ensureServerParams, err := cmdutil.NewEnsureServerParams(agentConfig)
	if err != nil {
		return err
	}
	err = cmdutil.EnsureMongoServer(ensureServerParams)
	if err != nil {
		return err
	}

	peerAddr := mongo.SelectPeerAddress(addrs)
	if peerAddr == "" {
		return fmt.Errorf("no appropriate peer address found in %q", addrs)
	}
	peerHostPort := net.JoinHostPort(peerAddr, fmt.Sprint(servingInfo.StatePort))

	if err := initiateMongoServer(peergrouper.InitiateMongoParams{
		DialInfo:       dialInfo,
		MemberHostPort: peerHostPort,
	}); err != nil {
		return err
	}
	logger.Infof("started mongo")
	return nil
}

// populateTools stores uploaded tools in provider storage
// and updates the tools metadata.
func (c *BootstrapCommand) populateTools(st *state.State, env environs.Environ) error {
	agentConfig := c.CurrentConfig()
	dataDir := agentConfig.DataDir()

	hostSeries, err := series.HostSeries()
	if err != nil {
		return errors.Trace(err)
	}
	current := version.Binary{
		Number: jujuversion.Current,
		Arch:   arch.HostArch(),
		Series: hostSeries,
	}
	tools, err := agenttools.ReadTools(dataDir, current)
	if err != nil {
		return errors.Trace(err)
	}

	data, err := ioutil.ReadFile(filepath.Join(
		agenttools.SharedToolsDir(dataDir, current),
		"tools.tar.gz",
	))
	if err != nil {
		return errors.Trace(err)
	}

	toolstorage, err := st.ToolsStorage()
	if err != nil {
		return errors.Trace(err)
	}
	defer toolstorage.Close()

	var toolsVersions []version.Binary
	if strings.HasPrefix(tools.URL, "file://") {
		// Tools were uploaded: clone for each series of the same OS.
		os, err := series.GetOSFromSeries(tools.Version.Series)
		if err != nil {
			return errors.Trace(err)
		}
		osSeries := series.OSSupportedSeries(os)
		for _, series := range osSeries {
			toolsVersion := tools.Version
			toolsVersion.Series = series
			toolsVersions = append(toolsVersions, toolsVersion)
		}
	} else {
		// Tools were downloaded from an external source: don't clone.
		toolsVersions = []version.Binary{tools.Version}
	}

	for _, toolsVersion := range toolsVersions {
		metadata := binarystorage.Metadata{
			Version: toolsVersion.String(),
			Size:    tools.Size,
			SHA256:  tools.SHA256,
		}
		logger.Debugf("Adding tools: %v", toolsVersion)
		if err := toolstorage.Add(bytes.NewReader(data), metadata); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// populateGUIArchive stores the uploaded Juju GUI archive in provider storage,
// updates the GUI metadata and set the current Juju GUI version.
func (c *BootstrapCommand) populateGUIArchive(st *state.State, env environs.Environ) error {
	agentConfig := c.CurrentConfig()
	dataDir := agentConfig.DataDir()
	guistorage, err := st.GUIStorage()
	if err != nil {
		return errors.Trace(err)
	}
	defer guistorage.Close()
	gui, err := agenttools.ReadGUIArchive(dataDir)
	if err != nil {
		return errors.Annotate(err, "cannot fetch GUI info")
	}
	f, err := os.Open(filepath.Join(agenttools.SharedGUIDir(dataDir), "gui.tar.bz2"))
	if err != nil {
		return errors.Annotate(err, "cannot read GUI archive")
	}
	defer f.Close()
	if err := guistorage.Add(f, binarystorage.Metadata{
		Version: gui.Version.String(),
		Size:    gui.Size,
		SHA256:  gui.SHA256,
	}); err != nil {
		return errors.Annotate(err, "cannot store GUI archive")
	}
	if err = st.GUISetVersion(gui.Version); err != nil {
		return errors.Annotate(err, "cannot set current GUI version")
	}
	return nil
}

// Override for testing.
var seriesFromVersion = series.VersionSeries

// saveCustomImageMetadata stores the custom image metadata to the database,
func (c *BootstrapCommand) saveCustomImageMetadata(st *state.State, env environs.Environ, imageMetadata []*imagemetadata.ImageMetadata) error {
	logger.Debugf("saving custom image metadata")
	return storeImageMetadataInState(st, env, "custom", simplestreams.CUSTOM_CLOUD_DATA, imageMetadata)
}

// storeImageMetadataInState writes image metadata into state store.
func storeImageMetadataInState(st *state.State, env environs.Environ, source string, priority int, existingMetadata []*imagemetadata.ImageMetadata) error {
	if len(existingMetadata) == 0 {
		return nil
	}
	cfg := env.Config()
	metadataState := make([]cloudimagemetadata.Metadata, len(existingMetadata))
	for i, one := range existingMetadata {
		m := cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          one.Stream,
				Region:          one.RegionName,
				Arch:            one.Arch,
				VirtType:        one.VirtType,
				RootStorageType: one.Storage,
				Source:          source,
				Version:         one.Version,
			},
			Priority: priority,
			ImageId:  one.Id,
		}
		s, err := seriesFromVersion(one.Version)
		if err != nil {
			return errors.Annotatef(err, "cannot determine series for version %v", one.Version)
		}
		m.Series = s
		if m.Stream == "" {
			m.Stream = cfg.ImageStream()
		}
		if m.Source == "" {
			m.Source = "custom"
		}
		metadataState[i] = m
	}
	if err := st.CloudImageMetadataStorage.SaveMetadata(metadataState); err != nil {
		return errors.Annotatef(err, "cannot cache image metadata")
	}
	return nil
}
