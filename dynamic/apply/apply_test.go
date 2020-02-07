/*
Copyright 2018 Google Inc.

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

package apply

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/json"
)

func TestMerge(t *testing.T) {
	table := []struct {
		name, observed, lastApplied, desired, want string
	}{
		{
			name:        "empty",
			observed:    `{}`,
			lastApplied: `{}`,
			desired:     `{}`,
			want:        `{}`,
		},
		{
			name:        "scalars",
			observed:    `{"a": "old", "b": "old", "c": "old"}`,
			lastApplied: `{"b": "old", "c": "old"}`,
			desired:     `{"c": "new", "d": "new" }`,
			want:        `{"c": "new", "d": "new", "a": "old"}`,
		},
		{
			name:        "scalars minus last applied",
			observed:    `{"a": "old", "b": "old", "c": "old"}`,
			lastApplied: `{}`,
			desired:     `{"a": "new", "d": "new" }`,
			want:        `{"a": "new", "d": "new", "b": "old", "c": "old"}`,
		},
		{
			name:        "scalars with last applied equals desired",
			observed:    `{"a": "old", "b": "old", "c": "old"}`,
			lastApplied: `{"a": "new", "d": "new"}`,
			desired:     `{"a": "new", "d": "new"}`,
			want:        `{"a": "new", "b": "old", "c": "old", "d": "new"}`,
		},
		{
			name:        "nested object",
			observed:    `{"hey": {"a": "old", "b": "old"}}`,
			lastApplied: `{"hey": {"b": "old", "a": "old"}}`,
			desired:     `{"hey": {"a": "new", "c": "new"}}`,
			want:        `{"hey": {"a": "new", "c": "new"}}`,
		},
		{
			name:        "nested object minus last applied",
			observed:    `{"hey": {"a": "old", "b": "old"}}`,
			lastApplied: `{}`,
			desired:     `{"hey": {"a": "new", "c": "new"}}`,
			want:        `{"hey": {"a": "new", "b": "old", "c": "new"}}`,
		},
		{
			name:        "nested object with last applied equals desired",
			observed:    `{"hey": {"a": "old", "b": "old"}}`,
			lastApplied: `{"hey": {"a": "new", "c": "new"}}`,
			desired:     `{"hey": {"a": "new", "c": "new"}}`,
			want:        `{"hey": {"a": "new", "b": "old", "c": "new"}}`,
		},
		{
			name:        "replace list",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{"list": [4,5,6]}`,
			desired:     `{"list": [7,8,9,{"b":false}]}`,
			want:        `{"list": [7,8,9,{"b":false}]}`,
		},
		{
			name:        "replace list minus last applied",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{}`,
			desired:     `{"list": [7,8,9,{"b":false}]}`,
			want:        `{"list": [7,8,9,{"b":false}]}`,
		},
		{
			name:        "replace list with last applied equals desired",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{"list": [7,8,9,{"b":false}]}`,
			desired:     `{"list": [7,8,9,{"b":false}]}`,
			want:        `{"list": [7,8,9,{"b":false}]}`,
		},
		{
			name:        "remove list minus last applied",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{}`,
			desired:     `{"list": []}`,
			want:        `{"list": []}`,
		},
		{
			name:        "remove list with last applied",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{"list": [7,8,9,{"b":false}]}`,
			desired:     `{"list": []}`,
			want:        `{"list": []}`,
		},
		{
			name:        "remove object with last applied minus desired",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{"list": [7,8,9,{"b":false}]}`,
			desired:     `{}`,
			want:        `{}`,
		},
		{
			name:        "keep list minus last applied minus desired",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{}`,
			desired:     `{}`,
			want:        `{"list": [1,2,3,{"a":true}]}`,
		},
		{
			name:        "remove specific items from list",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{"list": [2,{"a":true}]}`,
			desired:     `{"list": [3,{"a":true}]}`,
			want:        `{"list": [3,{"a":true}]}`,
		},
		{
			name:        "remove specific items from list minus last applied",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{}`,
			desired:     `{"list": [3,{"a":true}]}`,
			want:        `{"list": [3,{"a":true}]}`,
		},
		{
			name:        "remove specific items from list with empty last applied",
			observed:    `{"list": [1,2,3,{"a":true}]}`,
			lastApplied: `{"list": []}`,
			desired:     `{"list": [3,{"a":true}]}`,
			want:        `{"list": [3,{"a":true}]}`,
		},
		{
			name: "merge list-map",
			observed: `{
				"listMap": [
					{"name": "keep", "value": "other"},
					{"name": "remove", "value": "other"},
					{"name": "merge", "nested": {"keep": "other"}}
				],
				"ports1": [{"port": 80, "keep": "other"}],
				"ports2": [{"containerPort": 80, "keep": "other"}]
      		}`,
			lastApplied: `{
				"listMap": [{"name": "remove", "value": "old"}],
				"ports1": [{"port": 80, "remove": "old"}]
		    }`,
			desired: `{
				"listMap": [
					{"name": "add", "value": "new"},
					{"name": "merge", "nested": {"add": "new"}}
				],
				"ports1": [
					{"port": 80, "add": "new"},
					{"port": 90}
				],
				"ports2": [
					{"containerPort": 80},
					{"containerPort": 90}
				]
      		}`,
			want: `{
				"listMap": [
					{"name": "keep", "value": "other"},
					{"name": "merge", "nested": {"keep": "other", "add": "new"}},
					{"name": "add", "value": "new"}
				],
				"ports1": [
					{"port": 80, "keep": "other", "add": "new"},
					{"port": 90}
				],
				"ports2": [
					{"containerPort": 80, "keep": "other"},
					{"containerPort": 90}
				]
      		}`,
		},
		{
			name: "replace list of objects that's not a list-map",
			observed: `{
				"notListMap": [
					{"name": "keep", "value": "other"},
					{"notName": "remove", "value": "other"},
					{"name": "merge", "nested": {"keep": "other"}}
				]
			}`,
			lastApplied: `{
				"notListMap": [{"name": "remove", "value": "old"}]
      		}`,
			desired: `{
				"notListMap": [
					{"name": "add", "value": "new"},
					{"name": "merge", "nested": {"add": "new"}}
				]
      		}`,
			want: `{
				"notListMap": [
					{"name": "add", "value": "new"},
					{"name": "merge", "nested": {"add": "new"}}
				]
			}`,
		},
		//
		// test labels
		//
		{
			name: "labels with no changes && no last applied",
			observed: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
		},
		{
			name: "labels with no changes && with last applied",
			observed: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
		},
		{
			name: "labels with updates, removals & additions && no last applied",
			observed: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"labels": {
						"type": "simple storage",
						"ssd": "true"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "simple storage",
						"ssd": "true"
					}
				}
			}`,
		},
		{
			name: "labels with updates, removals & additions && with last applied",
			observed: `{
				"metadata": {
					"labels": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{
				"metadata": {
					"labels": {
						"name": "app"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"labels": {
						"type": "simple storage",
						"ssd": "true"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"labels": {
						"type": "simple storage",
						"ssd": "true"
					}
				}
			}`,
		},
		//
		// test annotations
		//
		{
			name: "annotations with no changes && no last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
		},
		{
			name: "annotations with no changes && with last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
		},
		{
			name: "annotations with updates, removals & addtions && no last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"annotations": {
						"type": "simple storage",
						"ssd": "true"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "simple storage",
						"ssd": "true"
					}
				}
			}`,
		},
		{
			name: "annotations with updates, removals & additions && with last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{
				"metadata": {
					"annotations": {
						"name": "app"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {
						"type": "simple storage",
						"ssd": "true"
					}
				}
      		}`,
			want: `{
				"metadata": {
					"annotations": {
						"type": "simple storage",
						"ssd": "true"
					}
				}
			}`,
		},
		{
			name: "empty annotations && no last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"annotations": {}
				}
      		}`,
			want: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
		},
		{
			name: "empty annotations && last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {
					"annotations": {}
				}
      		}`,
			want: `{
				"metadata": {
					"annotations": {}
				}
			}`,
		},
		{
			name: "nil annotations && no last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {}
      		}`,
			want: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
		},
		{
			name: "nil annotations && last applied",
			observed: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			lastApplied: `{
				"metadata": {
					"annotations": {
						"name": "app",
						"type": "storage"
					}
				}
			}`,
			desired: `{
				"metadata": {}
      		}`,
			want: `{
				"metadata": {}
			}`,
		},

		//
		// test finalizers i.e. list of strings
		// As per the logic desired will win always
		//
		{
			name: "finalizers - observed == desired && no last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
      		}`,
			want: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
		},
		{
			name: "finalizers - observed == desired == last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
      		}`,
			want: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
		},
		{
			name: "finalizers - desired has old, remove & add && no last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"app-two-protect",
						"storage-protect"
					]
				}
      		}`,
			want: `{
				"metadata": {
					"finalizers": [
						"app-two-protect",
						"storage-protect"
					]
				}
			}`,
		},
		{
			name: "finalizers - desired with old, remove & add && with last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{
				"metadata": {
					"finalizers": [
						"app-protect"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": [
						"app-two-protect",
						"storage-protect"
					]
				}
      		}`,
			want: `{
				"metadata": {
					"finalizers": [
						"app-two-protect",
						"storage-protect"
					]
				}
			}`,
		},
		{
			name: "finalizers - desired = empty && no last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {
					"finalizers": []
				}
      		}`,
			want: `{
				"metadata": {
					"finalizers": []
				}
			}`,
		},
		{
			name: "finalizers - desired = empty && last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			desired: `{
				"metadata": {
					"finalizers": []
				}
      		}`,
			want: `{
				"metadata": {
					"finalizers": []
				}
			}`,
		},
		{
			name: "finalizers - desired = nil && no last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"metadata": {}
      		}`,
			want: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
		},
		{
			name: "finalizers - desired = nil && last applied",
			observed: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			lastApplied: `{
				"metadata": {
					"finalizers": [
						"app-protect",
						"storage-protect"
					]
				}
			}`,
			desired: `{
				"metadata": {}
      		}`,
			want: `{
				"metadata": {}
			}`,
		},
		//
		// k8s style - list map - i.e. an array of maps
		//
		// NOTE:
		// 	At-least one of the field across the lists should be a
		// **known merge key** to consider the list as list of maps
		//
		// NOTE:
		//	List(s) with known merge key get executed as mergeListMap
		//
		{
			name: "list map - desired = nil && last applied = nil",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"spec": {}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
		},
		{
			name: "list map - desired = empty && last applied = nil",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
		},
		{
			name: "list map - desired = empty && last applied = empty",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": []
				}
			}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
		},
		{
			name: "list map - desired = empty && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": []
				}
			}`,
		},
		{
			name: "list map - observed = empty && desired = empty && last applied",
			observed: `{
				"spec": {
					"nodes": []
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": []
				}
			}`,
		},
		{
			name: "list map - observed = empty && desired && last applied",
			observed: `{
				"spec": {
					"nodes": []
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed = empty && desired && last applied = empty",
			observed: `{
				"spec": {
					"nodes": []
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": []
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed = nil && desired && last applied = nil",
			observed: `{
				"spec": {}
			}`,
			lastApplied: `{
				"spec": {}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101",
							"desc": "blah"
						},
						{
							"name": "node-201",
							"desc": "blah"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed = different order && desired = add && last applied = nil",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101"
						},
						{
							"name": "node-201"
						},
						{
							"name": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						},
						{
							"name": "node-301"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed && desired = remove, add && last applied = nil",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						},
						{
							"name": "node-301"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed && desired = remove, add && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-301"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed = different order && desired && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101"
						},
						{
							"name": "node-201"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
		},
		{
			name: "list map - observed = different order && desired = remove && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"name": "node-401"
						},
						{
							"name": "node-301"
						},
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"name": "node-401"
						},
						{
							"name": "node-301"
						},
						{
							"name": "node-201"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"name": "node-101"
						},
						{
							"name": "node-401"
						},
						{
							"name": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"name": "node-401"
						},
						{
							"name": "node-301"
						},
						{
							"name": "node-101"
						}
					]
				}
			}`,
		},
		//
		// list map - Unknown Merge Key - umk
		//
		// NOTE:
		//	List(s) with unknown merge key gets executed as mergeArray
		//
		{
			// Hint: this gets executed as mergeMap
			name: "list map - umk - desired = {} && last applied = {}",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"spec": {}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - desired = [] && last applied = {}",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			lastApplied: `{}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": []
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - desired = [] && last applied = []",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": []
				}
			}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": []
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - desired = [] && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": []
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed = [] && desired = [] && last applied",
			observed: `{
				"spec": {
					"nodes": []
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": []
				}
      		}`,
			want: `{
				"spec": {
					"nodes": []
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed = [] && desired && last applied",
			observed: `{
				"spec": {
					"nodes": []
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed = [] && desired && last applied = []",
			observed: `{
				"spec": {
					"nodes": []
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": []
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeMap
			name: "list map - umk - observed = {} && desired && last applied = {}",
			observed: `{
				"spec": {}
			}`,
			lastApplied: `{
				"spec": {}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101",
							"desc": "blah-101"
						},
						{
							"nodeName": "node-201",
							"desc": "blah-201"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed = different order && desired = add && last applied = {}",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101"
						},
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101"
						},
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed && desired = remove & add && last applied = {}",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed && desired = remove & add && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk - observed = different order && desired && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101"
						},
						{
							"nodeName": "node-201"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101"
						},
						{
							"nodeName": "node-201"
						}
					]
				}
			}`,
		},
		{
			// Hint: this gets executed as mergeArray
			name: "list map - umk- observed = different order && desired = remove && last applied = observed",
			observed: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-401"
						},
						{
							"nodeName": "node-301"
						},
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			lastApplied: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-401"
						},
						{
							"nodeName": "node-301"
						},
						{
							"nodeName": "node-201"
						},
						{
							"nodeName": "node-101"
						}
					]
				}
			}`,
			desired: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101"
						},
						{
							"nodeName": "node-401"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
      		}`,
			want: `{
				"spec": {
					"nodes": [
						{
							"nodeName": "node-101"
						},
						{
							"nodeName": "node-401"
						},
						{
							"nodeName": "node-301"
						}
					]
				}
			}`,
		},
	}

	for _, tc := range table {
		observed := make(map[string]interface{})
		if err := json.Unmarshal([]byte(tc.observed), &observed); err != nil {
			t.Errorf("%s: Can't unmarshal tc.observed: %v", tc.name, err)
			continue
		}
		lastApplied := make(map[string]interface{})
		if err := json.Unmarshal([]byte(tc.lastApplied), &lastApplied); err != nil {
			t.Errorf("%s: Can't unmarshal tc.lastApplied: %v", tc.name, err)
			continue
		}
		desired := make(map[string]interface{})
		if err := json.Unmarshal([]byte(tc.desired), &desired); err != nil {
			t.Errorf("%s: Can't unmarshal tc.desired: %v", tc.name, err)
			continue
		}
		want := make(map[string]interface{})
		if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
			t.Errorf("%s: Can't unmarshal tc.want: %v", tc.name, err)
			continue
		}

		got, err := Merge(observed, lastApplied, desired)
		if err != nil {
			t.Errorf("%s: Merge error: %v", tc.name, err)
			continue
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\nGot: %v\nWant: %v\nDiff: %s",
				tc.name, got, want, diff.ObjectReflectDiff(got, want),
			)
		}
	}
}

func TestLastAppliedAnnotation(t *testing.T) {
	// Round-trip some JSON through Set/Get methods.
	inJSON := `{
		"testing": "123"
	}`
	var in map[string]interface{}
	if err := json.Unmarshal([]byte(inJSON), &in); err != nil {
		t.Fatalf("can't unmarshal input: %v", err)
	}
	obj := &unstructured.Unstructured{}
	if err := SetLastApplied(obj, in); err != nil {
		t.Fatalf("SetLastApplied error: %v", err)
	}
	out, err := GetLastApplied(obj)
	if err != nil {
		t.Fatalf("GetLastApplied error: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("got %#v, want %#v", out, in)
	}
}
