// Copyright 2020 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The Qemu Machine Protocol - to remotely query and operate a qemu instance (https://wiki.qemu.org/Documentation/QMP)

package platform

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/coreos/mantle/util"
	"github.com/pkg/errors"

	"github.com/digitalocean/go-qemu/qmp"
)

// QOMDev is a QMP monitor, for interactions with a QEMU instance.
type QOMDev struct {
	Return []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"return"`
}

type QOMBlkDev struct {
	Return []struct {
		Device     string `json:"device"`
		DevicePath string `json:"qdev"`
		Removable  bool   `json:"removable"`
		Inserted   struct {
			BackingFileDepth int `json:"backing_file_depth"`
		} `json:"inserted"`
	} `json:"return"`
}

// qmpCommand is used to run thread-safe commands around using qmp.Socket.
type qmpCommand struct {
	socketPath string
	mu         sync.Mutex
}

// qemuSocket is global map that contains the sockets that have been opened
var qemuSockets = make(map[string]*qmpCommand)

// getQmpMonitor will either create a new qemuSocketMonitor OR return
// one that has already been created.
func getQmpMonitor(sockaddr string) (*qmpCommand, error) {
	qmpPath := filepath.Join(sockaddr, "qmp.sock")
	q, found := qemuSockets[qmpPath]
	if found {
		return q, nil
	}

	q = &qmpCommand{
		socketPath: qmpPath,
		mu:         sync.Mutex{},
	}
	qemuSockets[qmpPath] = q

	return q, nil
}

// run is a thread safe wrapper that ensures that only only one
// query/command is executed on the qemu qmp socket.
func (qsm *qmpCommand) run(command string) ([]byte, error) {
	qsm.mu.Lock()
	defer qsm.mu.Unlock()

	var monitor *qmp.SocketMonitor
	if err := util.Retry(10, 1*time.Second, func() error {
		m, err := qmp.NewSocketMonitor("unix", qsm.socketPath, 2*time.Second)
		if err != nil {
			return err
		}
		monitor = m
		return nil
	}); err != nil {
		return nil, errors.Wrapf(err, "Connecting to qemu monitor")
	}

	if err := monitor.Connect(); err != nil {
		return nil, fmt.Errorf("unable to connect to qemu socket: %w", err)
	}
	defer monitor.Disconnect()

	return monitor.Run([]byte(command))
}

// listQMPDevices executes a query which provides the list of devices and their names
func (qsm *qmpCommand) listQMPDevices() (*QOMDev, error) {
	listcmd := `{ "execute": "qom-list", "arguments": { "path": "/machine/peripheral-anon" } }`

	out, err := qsm.run(listcmd)
	if err != nil {
		return nil, errors.Wrapf(err, "Running QMP list command")
	}

	var devs QOMDev
	if err = json.Unmarshal(out, &devs); err != nil {
		return nil, errors.Wrapf(err, "De-serializing QMP output")
	}
	return &devs, nil
}

// listQMPBlkDevices executes a query which provides the list of block devices and their names
func (qsm *qmpCommand) listQMPBlkDevices() (*QOMBlkDev, error) {
	listcmd := `{ "execute": "query-block" }`

	out, err := qsm.run(listcmd)
	if err != nil {
		return nil, errors.Wrapf(err, "Running QMP list command")
	}

	var devs QOMBlkDev
	if err = json.Unmarshal(out, &devs); err != nil {
		return nil, errors.Wrapf(err, "De-serializing QMP output")
	}
	return &devs, nil
}

// setBootIndexForDevice set the bootindex for the particular device
func (qsm *qmpCommand) setBootIndexForDevice(device string, bootindex int) error {
	cmd := fmt.Sprintf(
		`{ "execute":"qom-set", "arguments": { "path":"%s", "property":"bootindex", "value":%d } }`,
		device, bootindex,
	)
	if _, err := qsm.run(cmd); err != nil {
		return errors.Wrapf(err, "Running QMP command")
	}
	return nil
}

// deleteBlockDevice deletes a block device for the particular qemu instance
func (qsm *qmpCommand) deleteBlockDevice(device string) error {
	cmd := fmt.Sprintf(`{ "execute": "device_del", "arguments": { "id":"%s" } }`, device)
	if _, err := qsm.run(cmd); err != nil {
		return errors.Wrapf(err, "Running QMP command")
	}
	return nil
}
