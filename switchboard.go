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
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

/* Try out injection instead of simple factory
type (
	Connector interface {
		Attach(AttachRequest) AttachResponse
		//Detach(args interface{})
		//getDevice(volID string) string
	}
	connectorFactory func() Connector
)

var New cFactory
*/

// Playing with logging approaches here, native log pkg is nice becuase it's light weight and simple
// end up adding level here and the ability to modify destination.  Maybe just use glog but thought
// I'd try something a little different here
var (
	// Trace sets a variable for Trace level logging
	Trace *log.Logger
	// Info sets a variable for Info level logging
	Info *log.Logger
	// Warning sets a variable for Warning level logging
	Warning *log.Logger
	// Error sets a variable for Error level logging
	Error *log.Logger
)

func init() {
	Trace = log.New(os.Stdout,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(os.Stdout,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(os.Stdout,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(os.Stdout,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

// Connector is a generic receiver for makign volume connections
type Connector interface {
	Connect(ConnectRequest) (ConnectResponse, error)
}

// ConnectRequest provides a wrapper for the specific driver/connection-type struct
type ConnectRequest struct {
	Type          string
	DriverRequest interface{}
}

// ConnectResponse includes response data from connect request including path and
// detailed driver specific info
type ConnectResponse struct {
	Path           string
	MPDevice       string
	BlkDevice      string
	DriverResponse interface{}
}

func stat(f string) (string, error) {
	out, err := exec.Command("sh", "-c", (fmt.Sprintf("stat %s", f))).CombinedOutput()
	return string(out), err

}

func waitForPathToExist(d string, maxRetries int) bool {
	for i := 0; i < maxRetries; i++ {
		if _, err := stat(d); err == nil {
			return true
		}
		time.Sleep(time.Second * time.Duration(2*1))
	}
	return false
}

func isMultipath(d string) bool {
	args := []string{"-c", d}
	out, err := exec.Command("multipath", args...).CombinedOutput()
	if err != nil {
		Error.Println("multipath check failed, multipath not running?")
		return false
	}
	Trace.Printf("response from multipath cmd: %s", out)
	if strings.Contains(string(out), "is a valid multipath device") {
		Trace.Printf("returning isMultipath == true\n")
		return true
	}
	return false
}

// New connector
func New(t string) Connector {
	switch t {
	case "iscsi":
		c, err := NewISCSIConnector()
		if err != nil {
			Error.Println("UR F'd")
		}
		return c
		// case "rbd", case "fc" etc etc
	}
	return nil
}

// Mount peforms mount
func Mount() {
}

// Unmount performs unmount operation
func Unmount() {
}

// GetBlkDevice Attempts to find the Block Device file for
// the specified raw device (including multipath).  Note that
// this is specific for SCSI devices.
func GetBlkDevice(d string) (string, error) {
	blkDev := ""
	args := []string{"-t"}
	out, err := exec.Command("lsscsi", args...).CombinedOutput()
	if err != nil {
		Error.Printf("unable to perform lsscsi -t, error: %+v", err)
		return blkDev, err
	}

	for _, entry := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.Contains(entry, d) {
			fields := strings.Fields(entry)
			blkDev = fields[len(fields)-1]
		}
	}
	if blkDev == "" {
		Error.Println("unable to find specified block file for the specified device")
		err := fmt.Errorf("unable to find lsscsi output for: %s", d)
		return "", err
	}

	if isMultipath(blkDev) == true {
		Info.Println("multipath detected...")
		args = []string{blkDev, "-n", "-o", "name", "-r"}
		out, err = exec.Command("lsblk", args...).CombinedOutput()
		if err != nil {
			Error.Printf("unable to find mpath device due to lsblk error: %+v\n", err)
			return blkDev, err
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 1 {
			mpdev := lines[1]
			blkDev = "/dev/mapper/" + mpdev
			Info.Printf("parsed %s to extract mp device: %s\n", lines, blkDev)

		} else {
			Error.Printf("unable to parse lsblk output (%v)\n", lines)
			// FIXME(jdg): Create an error here and return it
		}

	}
	return blkDev, nil
}
