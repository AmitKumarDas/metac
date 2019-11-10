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
	"io/ioutil"
	"strings"

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
	glog.V(4).Infof("Will load metac config(s) from path %s", c.Path)

	files, readDirErr := ioutil.ReadDir(c.Path)
	if readDirErr != nil {
		return nil, readDirErr
	}

	if len(files) == 0 {
		return nil, errors.Errorf("No metac config(s) found at %s", c.Path)
	}

	var out MetacConfigs

	// there can be multiple config files
	for _, file := range files {
		fileName := file.Name()
		if file.IsDir() || file.Mode().IsDir() {
			glog.V(4).Infof(
				"Will skip metac config %s at path %s: Not a file", fileName, c.Path,
			)
			// we don't want to load directory
			continue
		}
		if !strings.HasSuffix(fileName, ".yaml") && !strings.HasSuffix(fileName, ".json") {
			glog.V(4).Infof(
				"Will skip metac config %s at path %s: Not yaml or json", fileName, c.Path,
			)
			// we support either proper yaml or json file only
			continue
		}

		fileNameWithPath := c.Path + fileName
		glog.V(4).Infof("Will load metac config %s", fileNameWithPath)

		contents, readFileErr := ioutil.ReadFile(fileNameWithPath)
		if readFileErr != nil {
			return nil, errors.Wrapf(
				readFileErr, "Failed to read metac config %s", fileNameWithPath,
			)
		}

		ul, loaderr := k8s.YAMLToUnstructuredSlice(contents)
		if loaderr != nil {
			loaderr = errors.Wrapf(loaderr, "Failed to load metac config %s", fileNameWithPath)
			return nil, loaderr
		}

		glog.V(4).Infof("Metac config %s loaded successfully", fileNameWithPath)
		out = append(out, ul...)
	}

	glog.V(4).Infof("Metac config(s) loaded successfully from path %s", c.Path)
	return out, nil
}
