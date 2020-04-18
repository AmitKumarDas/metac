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

package composite

import (
	"flag"
	"testing"

	"k8s.io/klog"
	"openebs.io/metac/test/integration/framework"
)

// TestMain will run only once when go test is invoked
// against this package. All the other Test* functions
// will be invoked via m.Run call.
//
// NOTE:
//	There can be only one TestMain function in the entire
// package
func TestMain(m *testing.M) {
	flag.Parse()

	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	// Sync the glog and klog flags.
	flag.CommandLine.VisitAll(func(f1 *flag.Flag) {
		f2 := klogFlags.Lookup(f1.Name)
		if f2 != nil {
			value := f1.Value.String()
			f2.Value.Set(value)
		}
	})

	// Pass m.Run function to framework which in turn
	// sets up a kubernetes environment & then invokes
	// m.Run.
	err := framework.StartCRDBasedMetac(m.Run)
	if err != nil {
		// Since this is an error we must to invoke os.Exit(1)
		// as per TestMain guidelines
		klog.Exitf("%+v", err)
	}

	defer klog.Flush()
}
