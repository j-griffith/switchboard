/*
Copyright 2017 John Griffith.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package switchboard

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type iscsiConnector struct {
	InitiatorIQNs []string
	IFace         string
	AccessGroupID string
	HostName      string
}

// IscsiConnectRequest provides details about the iSCSI volume we wish to connect
type IscsiConnectRequest struct {
	Portal       string
	AuthMethod   string
	ChapLogin    string
	ChapPassword string
	TargetIQN    string
	Lun          int
}

// IscsiConnectResponse provides iSCSI specific connect response info as member of general ConnectResponse struct
// (DriverResponse) currently we're not adding anything for iSCSI
type IscsiConnectResponse struct {
}

func getInitiators() ([]string, error) {
	var iqns []string
	out, err := exec.Command("cat", "/etc/iscsi/initiatorname.iscsi").CombinedOutput()
	if err != nil {
		Error.Printf("unable to gather initiator names: %v\n", err)
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "InitiatorName=") {
			iqns = append(iqns, strings.Split(l, "=")[1])
		}
	}
	return iqns, nil
}

func getHostName() (string, error) {
	out, err := exec.Command("hostname").CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// NewISCSIConnector creates a new connection receiver of type iSCSI
func NewISCSIConnector() (Connector, error) {
	// FIXME(jdg): For now we're going to force the iscsi Iface to default, however we'll need to modify things to allow this to be detected or set
	// config, arguments to New?
	iface := "default"
	out, err := exec.Command("iscsiadm", []string{"-m", "iface", "-I", iface, "-o", "show"}...).CombinedOutput()
	if err != nil {
		Error.Printf("iscsi unable to read from interface %s, error: %s", iface, string(out))
		return &iscsiConnector{}, err
	}

	iqns, err := getInitiators()
	hostName, err := getHostName()
	if err != nil {
		return nil, err
	}
	return &iscsiConnector{
		InitiatorIQNs: iqns,
		HostName:      hostName,
		IFace:         iface,
	}, nil
}

func (c *iscsiConnector) Connect(req ConnectRequest) (ConnectResponse, error) {
	r, ok := req.DriverRequest.(IscsiConnectRequest)
	if ok == false {
		fmt.Println("no joy")
	}
	resp := ConnectResponse{}
	resp.DriverResponse = IscsiConnectResponse{}

	// Make sure we're not already attached
	resp.Path = "/dev/disk/by-path/ip-" + r.Portal + "-iscsi-" + r.TargetIQN + "lun-" + strconv.Itoa(r.Lun)
	if waitForPathToExist(resp.Path, 0) == true {
		return resp, nil
	}

	if strings.ToLower(r.AuthMethod) == "chap" {
		err := LoginWithChap(r.TargetIQN, r.Portal, r.ChapLogin, r.ChapPassword, c.IFace)
		if err != nil {
			Error.Printf("error: %+v", err)
		}
	} else {
		err := Login(r.TargetIQN, r.Portal, c.IFace)
		if err != nil {
			Error.Printf("error: %+v", err)
		}
	}

	// Check multipath
	return resp, nil
}

// LoginWithChap performs the necessary iscsiadm commands to log in to the
// specified iSCSI target.  This wrapper will create a new node record, setup
// CHAP credentials and issue the login command
func LoginWithChap(tiqn, portal, username, password, iface string) error {
	args := []string{"-m", "node", "-T", tiqn, "-p", portal}
	createArgs := append(args, []string{"--interface", iface, "--op", "new"}...)

	if _, err := exec.Command("iscsiadm", createArgs...).CombinedOutput(); err != nil {
		return err
	}

	authMethodArgs := append(args, []string{"--op=update", "--name", "node.session.auth.authmethod", "--value=CHAP"}...)
	if out, err := exec.Command("iscsiadm", authMethodArgs...).CombinedOutput(); err != nil {
		Error.Printf("output of failed iscsiadm cmd: %+v\n", out)
		return err
	}

	authUserArgs := append(args, []string{"--op=update", "--name", "node.session.auth.username", "--value=" + username}...)
	if _, err := exec.Command("iscsiadm", authUserArgs...).CombinedOutput(); err != nil {
		return err
	}
	authPasswordArgs := append(args, []string{"--op=update", "--name", "node.session.auth.password", "--value=" + password}...)
	if _, err := exec.Command("iscsiadm", authPasswordArgs...).CombinedOutput(); err != nil {
		return err
	}

	// Finally do the login
	loginArgs := append(args, []string{"--login"}...)
	_, err := exec.Command("iscsiadm", loginArgs...).CombinedOutput()
	return err
}

// Login performs a simple iSCSI login (devices that do not use CHAP)
func Login(tiqn, portal, iface string) error {
	args := []string{"-m", "node", "-T", tiqn, "-p", portal}
	loginArgs := append(args, []string{"--login"}...)
	Trace.Printf("attempt login with args: %s", loginArgs)
	_, err := exec.Command("iscsiadm", loginArgs...).CombinedOutput()
	return err
}
