// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/parallel"
	"github.com/juju/utils/series"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/ssh"

	"github.com/juju/1.25-upgrade/juju2/agent"
	"github.com/juju/1.25-upgrade/juju2/cloudconfig"
	"github.com/juju/1.25-upgrade/juju2/cloudconfig/cloudinit"
	"github.com/juju/1.25-upgrade/juju2/cloudconfig/instancecfg"
	"github.com/juju/1.25-upgrade/juju2/cloudconfig/sshinit"
	"github.com/juju/1.25-upgrade/juju2/environs"
	"github.com/juju/1.25-upgrade/juju2/environs/config"
	"github.com/juju/1.25-upgrade/juju2/environs/imagemetadata"
	"github.com/juju/1.25-upgrade/juju2/environs/simplestreams"
	"github.com/juju/1.25-upgrade/juju2/instance"
	"github.com/juju/1.25-upgrade/juju2/network"
	"github.com/juju/1.25-upgrade/juju2/status"
	coretools "github.com/juju/1.25-upgrade/juju2/tools"
)

var logger = loggo.GetLogger("juju.provider.common")

// Bootstrap is a common implementation of the Bootstrap method defined on
// environs.Environ; we strongly recommend that this implementation be used
// when writing a new provider.
func Bootstrap(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	result, series, finalizer, err := BootstrapInstance(ctx, env, args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	bsResult := &environs.BootstrapResult{
		Arch:     *result.Hardware.Arch,
		Series:   series,
		Finalize: finalizer,
	}
	return bsResult, nil
}

// BootstrapInstance creates a new instance with the series of its choice,
// constrained to those of the available tools, and
// returns the instance result, series, and a function that
// must be called to finalize the bootstrap process by transferring
// the tools and installing the initial Juju controller.
// This method is called by Bootstrap above, which implements environs.Bootstrap, but
// is also exported so that providers can manipulate the started instance.
func BootstrapInstance(ctx environs.BootstrapContext, env environs.Environ, args environs.BootstrapParams,
) (_ *environs.StartInstanceResult, selectedSeries string, _ environs.BootstrapFinalizer, err error) {
	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.

	// First thing, ensure we have tools otherwise there's no point.
	if args.BootstrapSeries != "" {
		selectedSeries = args.BootstrapSeries
	} else {
		selectedSeries = config.PreferredSeries(env.Config())
	}
	availableTools, err := args.AvailableTools.Match(coretools.Filter{
		Series: selectedSeries,
	})
	if err != nil {
		return nil, "", nil, err
	}

	// Filter image metadata to the selected series.
	var imageMetadata []*imagemetadata.ImageMetadata
	seriesVersion, err := series.SeriesVersion(selectedSeries)
	if err != nil {
		return nil, "", nil, errors.Trace(err)
	}
	for _, m := range args.ImageMetadata {
		if m.Version != seriesVersion {
			continue
		}
		imageMetadata = append(imageMetadata, m)
	}

	// Get the bootstrap SSH client. Do this early, so we know
	// not to bother with any of the below if we can't finish the job.
	client := ssh.DefaultClient
	if client == nil {
		// This should never happen: if we don't have OpenSSH, then
		// go.crypto/ssh should be used with an auto-generated key.
		return nil, "", nil, fmt.Errorf("no SSH client available")
	}

	publicKey, err := simplestreams.UserPublicSigningKey()
	if err != nil {
		return nil, "", nil, err
	}
	envCfg := env.Config()
	instanceConfig, err := instancecfg.NewBootstrapInstanceConfig(
		args.ControllerConfig, args.BootstrapConstraints, args.ModelConstraints, selectedSeries, publicKey,
	)
	if err != nil {
		return nil, "", nil, err
	}
	instanceConfig.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
	instanceConfig.EnableOSUpgrade = env.Config().EnableOSUpgrade()
	instanceConfig.NetBondReconfigureDelay = env.Config().NetBondReconfigureDelay()

	instanceConfig.Tags = instancecfg.InstanceTags(envCfg.UUID(), args.ControllerConfig.ControllerUUID(), envCfg, instanceConfig.Jobs)
	maybeSetBridge := func(icfg *instancecfg.InstanceConfig) {
		// If we need to override the default bridge name, do it now. When
		// args.ContainerBridgeName is empty, the default names for LXC
		// (lxcbr0) and KVM (virbr0) will be used.
		if args.ContainerBridgeName != "" {
			logger.Debugf("using %q as network bridge for all container types", args.ContainerBridgeName)
			if icfg.AgentEnvironment == nil {
				icfg.AgentEnvironment = make(map[string]string)
			}
			icfg.AgentEnvironment[agent.LxcBridge] = args.ContainerBridgeName
		}
	}
	maybeSetBridge(instanceConfig)

	bootstrapMsg := env.BootstrapMessage()
	if bootstrapMsg != "" {
		ctx.Infof(bootstrapMsg)
	}

	cloudRegion := args.CloudName
	if args.CloudRegion != "" {
		cloudRegion += "/" + args.CloudRegion
	}
	fmt.Fprintf(ctx.GetStderr(), "Launching controller instance(s) on %s...\n", cloudRegion)
	// Print instance status reports status changes during provisioning.
	// Note the carriage returns, meaning subsequent prints are to the same
	// line of stderr, not a new line.
	instanceStatus := func(settableStatus status.Status, info string, data map[string]interface{}) error {
		// The data arg is not expected to be used in this case, but
		// print it, rather than ignore it, if we get something.
		dataString := ""
		if len(data) > 0 {
			dataString = fmt.Sprintf(" %v", data)
		}
		fmt.Fprintf(ctx.GetStderr(), " - %s%s\r", info, dataString)
		return nil
	}
	// Likely used after the final instanceStatus call to white-out the
	// current stderr line before the next use, removing any residual status
	// reporting output.
	statusCleanup := func(info string) error {
		// The leading spaces account for the leading characters
		// emitted by instanceStatus above.
		fmt.Fprintf(ctx.GetStderr(), "   %s\r", info)
		return nil
	}
	result, err := env.StartInstance(environs.StartInstanceParams{
		ControllerUUID:  args.ControllerConfig.ControllerUUID(),
		Constraints:     args.BootstrapConstraints,
		Tools:           availableTools,
		InstanceConfig:  instanceConfig,
		Placement:       args.Placement,
		ImageMetadata:   imageMetadata,
		StatusCallback:  instanceStatus,
		CleanupCallback: statusCleanup,
	})
	if err != nil {
		return nil, "", nil, errors.Annotate(err, "cannot start bootstrap instance")
	}

	msg := fmt.Sprintf(" - %s (%s)", result.Instance.Id(), formatHardware(result.Hardware))
	// We need some padding below to overwrite any previous messages.
	if len(msg) < 40 {
		padding := make([]string, 40-len(msg))
		msg += strings.Join(padding, " ")
	}
	fmt.Fprintln(ctx.GetStderr(), msg)

	finalize := func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, opts environs.BootstrapDialOpts) error {
		icfg.Bootstrap.BootstrapMachineInstanceId = result.Instance.Id()
		icfg.Bootstrap.BootstrapMachineHardwareCharacteristics = result.Hardware
		envConfig := env.Config()
		if result.Config != nil {
			updated, err := envConfig.Apply(result.Config.UnknownAttrs())
			if err != nil {
				return errors.Trace(err)
			}
			envConfig = updated
		}
		if err := instancecfg.FinishInstanceConfig(icfg, envConfig); err != nil {
			return err
		}
		maybeSetBridge(icfg)
		return FinishBootstrap(ctx, client, env, result.Instance, icfg, opts)
	}
	return result, selectedSeries, finalize, nil
}

func formatHardware(hw *instance.HardwareCharacteristics) string {
	if hw == nil {
		return ""
	}
	out := make([]string, 0, 3)
	if hw.Arch != nil && *hw.Arch != "" {
		out = append(out, fmt.Sprintf("arch=%s", *hw.Arch))
	}
	if hw.Mem != nil && *hw.Mem > 0 {
		out = append(out, fmt.Sprintf("mem=%s", formatMemory(*hw.Mem)))
	}
	if hw.CpuCores != nil && *hw.CpuCores > 0 {
		out = append(out, fmt.Sprintf("cores=%d", *hw.CpuCores))
	}
	return strings.Join(out, " ")
}

func formatMemory(m uint64) string {
	if m < 1024 {
		return fmt.Sprintf("%dM", m)
	}
	s := fmt.Sprintf("%.1f", float32(m)/1024.0)
	return strings.TrimSuffix(s, ".0") + "G"
}

// FinishBootstrap completes the bootstrap process by connecting
// to the instance via SSH and carrying out the cloud-config.
//
// Note: FinishBootstrap is exposed so it can be replaced for testing.
var FinishBootstrap = func(
	ctx environs.BootstrapContext,
	client ssh.Client,
	env environs.Environ,
	inst instance.Instance,
	instanceConfig *instancecfg.InstanceConfig,
	opts environs.BootstrapDialOpts,
) error {
	interrupted := make(chan os.Signal, 1)
	ctx.InterruptNotify(interrupted)
	defer ctx.StopInterruptNotify(interrupted)
	addr, err := WaitSSH(
		ctx.GetStderr(),
		interrupted,
		client,
		GetCheckNonceCommand(instanceConfig),
		&RefreshableInstance{inst, env},
		opts,
	)
	if err != nil {
		return err
	}
	return ConfigureMachine(ctx, client, addr, instanceConfig)
}

func GetCheckNonceCommand(instanceConfig *instancecfg.InstanceConfig) string {
	// Each attempt to connect to an address must verify the machine is the
	// bootstrap machine by checking its nonce file exists and contains the
	// nonce in the InstanceConfig. This also blocks sshinit from proceeding
	// until cloud-init has completed, which is necessary to ensure apt
	// invocations don't trample each other.
	nonceFile := utils.ShQuote(path.Join(instanceConfig.DataDir, cloudconfig.NonceFile))
	checkNonceCommand := fmt.Sprintf(`
	noncefile=%s
	if [ ! -e "$noncefile" ]; then
		echo "$noncefile does not exist" >&2
		exit 1
	fi
	content=$(cat $noncefile)
	if [ "$content" != %s ]; then
		echo "$noncefile contents do not match machine nonce" >&2
		exit 1
	fi
	`, nonceFile, utils.ShQuote(instanceConfig.MachineNonce))
	return checkNonceCommand
}

func ConfigureMachine(ctx environs.BootstrapContext, client ssh.Client, host string, instanceConfig *instancecfg.InstanceConfig) error {
	// Bootstrap is synchronous, and will spawn a subprocess
	// to complete the procedure. If the user hits Ctrl-C,
	// SIGINT is sent to the foreground process attached to
	// the terminal, which will be the ssh subprocess at this
	// point. For that reason, we do not call StopInterruptNotify
	// until this function completes.
	cloudcfg, err := cloudinit.New(instanceConfig.Series)
	if err != nil {
		return errors.Trace(err)
	}

	// Set packaging update here
	cloudcfg.SetSystemUpdate(instanceConfig.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(instanceConfig.EnableOSUpgrade)

	udata, err := cloudconfig.NewUserdataConfig(instanceConfig, cloudcfg)
	if err != nil {
		return err
	}
	if err := udata.ConfigureJuju(); err != nil {
		return err
	}
	configScript, err := cloudcfg.RenderScript()
	if err != nil {
		return err
	}
	script := shell.DumpFileOnErrorScript(instanceConfig.CloudInitOutputLog) + configScript
	return sshinit.RunConfigureScript(script, sshinit.ConfigureParams{
		Host:           "ubuntu@" + host,
		Client:         client,
		Config:         cloudcfg,
		ProgressWriter: ctx.GetStderr(),
		Series:         instanceConfig.Series,
	})
}

// InstanceRefresher is the subet of the Instance interface required
// for waiting for SSH access to become availble.
type InstanceRefresher interface {
	// Refresh refreshes the addresses for the instance.
	Refresh() error

	// Addresses returns the addresses for the instance.
	// To ensure that the results are up to date, call
	// Refresh first.
	Addresses() ([]network.Address, error)

	// Status returns the provider-specific status for the
	// instance.
	Status() instance.InstanceStatus
}

type RefreshableInstance struct {
	instance.Instance
	Env environs.Environ
}

// Refresh refreshes the addresses for the instance.
func (i *RefreshableInstance) Refresh() error {
	instances, err := i.Env.Instances([]instance.Id{i.Id()})
	if err != nil {
		return errors.Trace(err)
	}
	i.Instance = instances[0]
	return nil
}

type hostChecker struct {
	addr   network.Address
	client ssh.Client
	wg     *sync.WaitGroup

	// checkDelay is the amount of time to wait between retries.
	checkDelay time.Duration

	// checkHostScript is executed on the host via SSH.
	// hostChecker.loop will return once the script
	// runs without error.
	checkHostScript string

	// closed is closed to indicate that the host checker should
	// return, without waiting for the result of any ongoing
	// attempts.
	closed <-chan struct{}
}

// Close implements io.Closer, as required by parallel.Try.
func (*hostChecker) Close() error {
	return nil
}

func (hc *hostChecker) loop(dying <-chan struct{}) (io.Closer, error) {
	defer hc.wg.Done()
	// The value of connectSSH is taken outside the goroutine that may outlive
	// hostChecker.loop, or we evoke the wrath of the race detector.
	connectSSH := connectSSH
	done := make(chan error, 1)
	var lastErr error
	for {
		address := hc.addr.Value
		go func() {
			done <- connectSSH(hc.client, address, hc.checkHostScript)
		}()
		select {
		case <-dying:
			return hc, lastErr
		case lastErr = <-done:
			if lastErr == nil {
				return hc, nil
			}
			logger.Debugf("connection attempt for %s failed: %v", address, lastErr)
		}
		select {
		case <-hc.closed:
			return hc, lastErr
		case <-dying:
		case <-time.After(hc.checkDelay):
		}
	}
}

type parallelHostChecker struct {
	*parallel.Try
	client ssh.Client
	stderr io.Writer
	wg     sync.WaitGroup

	// active is a map of adresses to channels for addresses actively
	// being tested. The goroutine testing the address will continue
	// to attempt connecting to the address until it succeeds, the Try
	// is killed, or the corresponding channel in this map is closed.
	active map[network.Address]chan struct{}

	// checkDelay is how long each hostChecker waits between attempts.
	checkDelay time.Duration

	// checkHostScript is the script to run on each host to check that
	// it is the host we expect.
	checkHostScript string
}

func (p *parallelHostChecker) UpdateAddresses(addrs []network.Address) {
	for _, addr := range addrs {
		if _, ok := p.active[addr]; ok {
			continue
		}
		fmt.Fprintf(p.stderr, "Attempting to connect to %s:22\n", addr.Value)
		closed := make(chan struct{})
		hc := &hostChecker{
			addr:            addr,
			client:          p.client,
			checkDelay:      p.checkDelay,
			checkHostScript: p.checkHostScript,
			closed:          closed,
			wg:              &p.wg,
		}
		p.wg.Add(1)
		p.active[addr] = closed
		p.Start(hc.loop)
	}
}

// Close prevents additional functions from being added to
// the Try, and tells each active hostChecker to exit.
func (p *parallelHostChecker) Close() error {
	// We signal each checker to stop and wait for them
	// each to complete; this allows us to get the error,
	// as opposed to when using try.Kill which does not
	// wait for the functions to complete.
	p.Try.Close()
	for _, ch := range p.active {
		close(ch)
	}
	return nil
}

// connectSSH is called to connect to the specified host and
// execute the "checkHostScript" bash script on it.
var connectSSH = func(client ssh.Client, host, checkHostScript string) error {
	cmd := client.Command("ubuntu@"+host, []string{"/bin/bash"}, nil)
	cmd.Stdin = strings.NewReader(checkHostScript)
	output, err := cmd.CombinedOutput()
	if err != nil && len(output) > 0 {
		err = fmt.Errorf("%s", strings.TrimSpace(string(output)))
	}
	return err
}

// WaitSSH waits for the instance to be assigned a routable
// address, then waits until we can connect to it via SSH.
//
// waitSSH attempts on all addresses returned by the instance
// in parallel; the first succeeding one wins. We ensure that
// private addresses are for the correct machine by checking
// the presence of a file on the machine that contains the
// machine's nonce. The "checkHostScript" is a bash script
// that performs this file check.
func WaitSSH(
	stdErr io.Writer,
	interrupted <-chan os.Signal,
	client ssh.Client,
	checkHostScript string,
	inst InstanceRefresher,
	opts environs.BootstrapDialOpts,
) (addr string, err error) {
	globalTimeout := time.After(opts.Timeout)
	pollAddresses := time.NewTimer(0)

	// checker checks each address in a loop, in parallel,
	// until one succeeds, the global timeout is reached,
	// or the tomb is killed.
	checker := parallelHostChecker{
		Try:             parallel.NewTry(0, nil),
		client:          client,
		stderr:          stdErr,
		active:          make(map[network.Address]chan struct{}),
		checkDelay:      opts.RetryDelay,
		checkHostScript: checkHostScript,
	}
	defer checker.wg.Wait()
	defer checker.Kill()

	fmt.Fprintln(stdErr, "Waiting for address")
	for {
		select {
		case <-pollAddresses.C:
			pollAddresses.Reset(opts.AddressesDelay)
			if err := inst.Refresh(); err != nil {
				return "", fmt.Errorf("refreshing addresses: %v", err)
			}
			instanceStatus := inst.Status()
			if instanceStatus.Status == status.ProvisioningError {
				if instanceStatus.Message != "" {
					return "", errors.Errorf("instance provisioning failed (%v)", instanceStatus.Message)
				}
				return "", errors.Errorf("instance provisioning failed")
			}
			addresses, err := inst.Addresses()
			if err != nil {
				return "", fmt.Errorf("getting addresses: %v", err)
			}
			checker.UpdateAddresses(addresses)
		case <-globalTimeout:
			checker.Close()
			lastErr := checker.Wait()
			format := "waited for %v "
			args := []interface{}{opts.Timeout}
			if len(checker.active) == 0 {
				format += "without getting any addresses"
			} else {
				format += "without being able to connect"
			}
			if lastErr != nil && lastErr != parallel.ErrStopped {
				format += ": %v"
				args = append(args, lastErr)
			}
			return "", fmt.Errorf(format, args...)
		case <-interrupted:
			return "", fmt.Errorf("interrupted")
		case <-checker.Dead():
			result, err := checker.Result()
			if err != nil {
				return "", err
			}
			return result.(*hostChecker).addr.Value, nil
		}
	}
}
