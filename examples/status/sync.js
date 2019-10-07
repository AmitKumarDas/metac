/*
Copyright 2017 Google Inc.
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


module.exports = async function (context) {
  return {
    status: 200, 
    body: {
      // Noop resource will have this status set
      // after the sync operation
      status: { 
        message: "success",
      }, 
    }, 
    headers: {'Content-Type': 'application/json'},
  };
};
