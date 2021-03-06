// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"github.com/juju/version"

	coretools "github.com/juju/1.25-upgrade/juju2/tools"
)

var upgradeAgentsDoc = ` 

The purpose of the upgrade-agents command is to upgrade the agents on the 1.25
environment to the version used by the controller.

This command updates the tools symlinks for the agents, and updates their
agent config files to specify the correct version, along with the CA Cert and
addersses of the controller.

`

func newUpgradeAgentsCommand() cmd.Command {
	return &upgradeAgentsCommand{
		baseClientCommand{
			needsController: true,
			remoteCommand:   "upgrade-agents-impl",
		},
	}
}

type upgradeAgentsCommand struct {
	baseClientCommand
}

func (c *upgradeAgentsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-agents",
		Args:    "<environment name> <controller name>",
		Purpose: "upgrade all the agents for the specified environment",
		Doc:     upgradeAgentsDoc,
	}
}

func (c *upgradeAgentsCommand) Init(args []string) error {
	args, err := c.baseClientCommand.init(args)
	if err != nil {
		return errors.Trace(err)
	}
	return cmd.CheckEmpty(args)
}

var upgradeAgentsImplDoc = `

upgrade-agents-impl must be executed on an API server machine of a 1.25
environment.

The command will get a list of all the machines, and their addresses, and then
ssh to all the machines to upgrade the various agents on those machines.

`

func newUpgradeAgentsImplCommand() cmd.Command {
	return &upgradeAgentsImplCommand{
		baseRemoteCommand{needsController: true},
	}
}

type upgradeAgentsImplCommand struct {
	baseRemoteCommand
}

func (c *upgradeAgentsImplCommand) Init(args []string) error {
	args, err := c.baseRemoteCommand.init(args)
	if err != nil {
		return errors.Trace(err)
	}

	return cmd.CheckEmpty(args)
}

func (c *upgradeAgentsImplCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-agents-impl",
		Purpose: "controller aspect of upgrade-agents",
		Doc:     upgradeAgentsImplDoc,
	}
}

func (c *upgradeAgentsImplCommand) Run(ctx *cmd.Context) error {
	st, err := c.getState(ctx)
	if err != nil {
		return errors.Annotate(err, "getting state")
	}
	defer st.Close()

	// Here we always use the 1.25 environment to get all of the machine
	// addresses. We then use those to ssh into every one of those machine
	// and run the service status script against all the agents.
	machines, err := getMachines(st)
	if err != nil {
		return errors.Annotate(err, "unable to get addresses for machines")
	}

	_ = machines

	conn, err := c.getControllerConnection()
	if err != nil {
		return errors.Annotate(err, "getting controller connection")
	}
	defer conn.Close()

	ver, _ := conn.ServerVersion()
	fmt.Fprintf(ctx.Stdout, "Controller version: %s\n", ver)
	fmt.Fprintf(ctx.Stdout, "Controller addresses: %#v\n", conn.APIHostPorts())
	fmt.Fprintf(ctx.Stdout, "Controller UUID: %s\n", conn.ControllerTag().Id())

	// Make a dir to put the downloaded tools into.
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		return errors.Trace(err)
	}

	// Get a list of all the architectures and series for the machines?
	toolsNeeded := set.NewStrings()
	for _, m := range machines {
		binary := version.MustParseBinary(m.Tools)
		toolsNeeded.Add(fmt.Sprintf("%s-%s", binary.Series, binary.Arch))
	}

	// Get the tools from the controller.
	client := utils.GetNonValidatingHTTPClient()
	toolsURLPrefix := fmt.Sprintf("https://%s/tools/%s-", conn.Addr(), ver)
	for _, seriesArch := range toolsNeeded.SortedValues() {
		if err := c.getTools(ctx, client, ver, toolsURLPrefix, seriesArch); err != nil {
			return errors.Annotatef(err, "downloading tools %s-%s", ver, seriesArch)
		}
	}

	//

	return errors.Errorf("this command not yet finished")
}

func (c *upgradeAgentsImplCommand) getTools(ctx *cmd.Context, client *http.Client, ver version.Number, toolsURLPrefix, seriesArch string) error {
	toolsUrl := toolsURLPrefix + seriesArch
	toolsVersion := version.MustParseBinary(ver.String() + "-" + seriesArch)

	// Look to see if the directory is already there, if it is, assume
	// that it is good.
	downloadedToolsDir := path.Join(toolsDir, toolsVersion.String())
	if _, err := os.Stat(downloadedToolsDir); err == nil {
		fmt.Fprintf(ctx.Stdout, "%s exists\n", downloadedToolsDir)
		return nil
	}

	fmt.Fprintf(ctx.Stdout, "Downloading tools: %s\n", toolsUrl)
	resp, err := client.Get(toolsUrl)
	if err != nil {
		return errors.Annotate(err, "downloading tools")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("bad HTTP response: %v", resp.Status)
	}

	err = UnpackTools(toolsDir, toolsVersion, resp.Body)
	if err != nil {
		return errors.Errorf("cannot unpack tools: %v", err)
	}
	return nil
}

// UnpackTools reads a set of juju tools in gzipped tar-archive
// format and unpacks them into the appropriate tools directory
// within dataDir. If a valid tools directory already exists,
// UnpackTools returns without error.
func UnpackTools(dataDir string, toolsVersion version.Binary, r io.Reader) (err error) {
	// Unpack the gzip file and compute the checksum.
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer zr.Close()
	f, err := ioutil.TempFile(os.TempDir(), "tools-tar")
	if err != nil {
		return err
	}
	_, err = io.Copy(f, zr)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	// Make a temporary directory in the tools directory,
	// first ensuring that the tools directory exists.
	dir, err := ioutil.TempDir(toolsDir, "unpacking-")
	if err != nil {
		return err
	}
	defer removeAll(dir)

	// Checksum matches, now reset the file and untar it.
	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if strings.ContainsAny(hdr.Name, "/\\") {
			return fmt.Errorf("bad name %q in tools archive", hdr.Name)
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("bad file type %c in file %q in tools archive", hdr.Typeflag, hdr.Name)
		}
		name := path.Join(dir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return errors.Annotatef(err, "tar extract %q failed", name)
		}
	}
	// Write some metadata about the tools.
	tools := &coretools.Tools{Version: toolsVersion}
	toolsMetadataData, err := json.Marshal(tools)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path.Join(dir, toolsFile), []byte(toolsMetadataData), 0644)
	if err != nil {
		return err
	}

	// The tempdir is created with 0700, so we need to make it more
	// accessable for juju-run.
	err = os.Chmod(dir, 0755)
	if err != nil {
		return err
	}

	return os.Rename(dir, path.Join(toolsDir, toolsVersion.String()))
}

func removeAll(dir string) {
	err := os.RemoveAll(dir)
	if err == nil || os.IsNotExist(err) {
		return
	}
	logger.Errorf("cannot remove %q: %v", dir, err)
}

func writeFile(name string, mode os.FileMode, r io.Reader) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
