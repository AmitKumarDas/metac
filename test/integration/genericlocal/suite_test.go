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

package genericlocal

import (
	"testing"

	"openebs.io/metac/test/integration/framework"
)

// This will be run only once when go test is invoked against this
// package. All the other Test* functions will be invoked via m.Run
// call.
//
// NOTE:
// 	`func TestMain(m *testing.M) {...}`
// is the canonical golang way to execute the test functions
// present in all *_test.go files in this package.
//
// NOTE:
// 	framework.TestWithConfigMetac provides the common dependencies
// like setup & teardown to let this test package run properly.
//
// NOTE:
// 	Instead of directly invoking m.Run() where m is *testing.M this
// function delegates to framework's TestWithConfigMetac which in
// turn invokes m.Run()
func TestMain(m *testing.M) {
	framework.TestWithConfigMetac(m.Run)
}
