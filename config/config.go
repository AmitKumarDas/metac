/*
Copyright 2019 The MayaData Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/json"

	"openebs.io/metac/apis/metacontroller/v1alpha1"
	k8s "openebs.io/metac/third_party/kubernetes"
)

// MetacConfigs represents unmarshaled form of all
// the config files provided to run Metac controllers
type MetacConfigs []unstructured.Unstructured

// ListGenericControllers returns all GenericController configs
func (mc MetacConfigs) ListGenericControllers() ([]*v1alpha1.GenericController, error) {
	var gctls []*v1alpha1.GenericController
	for _, u := range mc {
		if u.GetKind() != "GenericController" {
			continue
		}
		raw, err := u.MarshalJSON()
		if err != nil {
			return nil, err
		}
		gctl := v1alpha1.GenericController{}
		if err := json.Unmarshal(raw, &gctl); err != nil {
			return nil, err
		}
		gctls = append(gctls, &gctl)
	}
	return gctls, nil
}

// Config is the path to metac's Config files
type Config struct {
	Path string
}

// New returns a new instance of config
func New(path string) *Config {
	return &Config{path}
}

// Load loads all metac config files & converts them
// to unstructured instances
func (c *Config) Load() (MetacConfigs, error) {
	files, readerr := ioutil.ReadDir(c.Path)
	if readerr != nil {
		return nil, readerr
	}

	var out MetacConfigs
	var loaderr error

	// there can be multiple config files
	for _, file := range files {
		f, openerr := os.Open(file.Name())
		if openerr != nil {
			glog.Errorf("Failed to open metac config file %s: %v", file.Name(), openerr)
			return nil, openerr
		}
		defer func() {
			if loaderr != nil {
				glog.Errorf("%v", loaderr)
			}
			if closeerr := f.Close(); closeerr != nil {
				glog.Fatal(closeerr)
			}
		}()

		var buffer bytes.Buffer
		s := bufio.NewScanner(f)
		for s.Scan() {
			buffer.Write(s.Bytes())
		}
		if loaderr = s.Err(); loaderr != nil {
			loaderr = errors.Wrapf(loaderr, "Failed to load metac config %s", file.Name())
			return nil, loaderr
		}

		ul, loaderr := k8s.YAMLToUnstructuredSlice(buffer.Bytes())
		if loaderr != nil {
			loaderr = errors.Wrapf(loaderr, "Failed to load metac config %s", file.Name())
			return nil, loaderr
		}
		out = append(out, ul...)
	}
	return out, nil
}
