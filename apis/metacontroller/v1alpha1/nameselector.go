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

package v1alpha1

// NameSelector is used to select resources based on
// the names set here
type NameSelector []string

// Contains returns true if the provided search item
// is present in the selector
func (s NameSelector) Contains(search string) bool {
	for _, name := range s {
		if name == search {
			return true
		}
	}
	return false
}

// ContainsOrTrue returns true if nameselector is empty or
// search item is available in nameselector
func (s NameSelector) ContainsOrTrue(search string) bool {
	if len(s) == 0 {
		return true
	}
	return s.Contains(search)
}
