// Copyright 2015 CoreOS, Inc.
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

package register

import "github.com/coreos/mantle/platform"

type Test struct {
	Name        string // should be uppercase and unique
	Run         func(platform.TestCluster) error
	NativeFuncs map[string]func() error
	UserData    string
	ClusterSize int
	Platforms   []string // whitelist of platforms to run test against -- defaults to all

	// If manual is set, the test will only execute if the name fully matches without globbing.
	Manual bool
}

// maps names to tests
var Tests = map[string]*Test{}

// panic if existing name is registered
func Register(t *Test) {
	_, ok := Tests[t.Name]
	if ok {
		panic("test already registered with same name")
	}
	Tests[t.Name] = t
}
