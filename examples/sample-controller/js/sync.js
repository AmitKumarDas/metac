/*
Copyright 2017 Google Inc.

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

var desiredDeployment = function (foo) {
  let lbls = {
    app: 'nginx',
    controller: foo.metadata.name
  }
  let deploy = {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name: foo.spec.deploymentName,
      namespace: foo.metadata.namespace
    },
    spec: {
      replicas: foo.spec.replicas,
      selector: {
        matchLabels: lbls
      },
      template: {
        metadata: {
          labels: lbls
        },
        spec: {
          containers: [
            {
              name: 'nginx',
              image: 'nginx:latest'
            }
          ]
        }
      }
    }
  };
  return deploy;
};

module.exports = async function (context) {
  let observed = context.request.body;
  let desired = {status: {}, children: []};

  try {
    // observed foo object
    let foo = observed.parent;

    // extract available replicas from desired deployment if available
    let allDeploys = observed.children['Deployment.apps/v1'];
    let fooDeploy = allDeploys ? allDeploys[foo.spec.deploymentName] : null;
    let replicas = fooDeploy ? fooDeploy.status.availableReplicas : 0;

    // Set the status of Foo
    desired.status = {
      availableReplicas: replicas
    };

    // Generate/Apply desired children
    desired.children = [
      desiredDeployment(foo)
    ];
  } catch (e) {
    return {status: 500, body: e.stack};
  }

  return {status: 200, body: desired, headers: {'Content-Type': 'application/json'}};
};
