function(request) {
  // Create a CRD for the namespace specified in GenericController
  attachments: [
    {
      apiVersion: "apiextensions.k8s.io/v1beta1",
      kind: "CustomResourceDefinition",
      metadata: {
        // name must match the spec fields below, and be in the form: 
        // <plural>.<group>
        name: "storages.dao.amitd.io",
      },
      spec: {
        // group name to use for REST API: /apis/<group>/<version>
        group: "dao.amitd.io",
        // version name to use for REST API: /apis/<group>/<version>
        version: "v1alpha1",
        // either Namespaced or Cluster
        scope: "Namespaced",
        names:{
          // plural name to be used in the URL: 
          // i.e. /apis/<group>/<version>/<plural>
          plural: "storages",
          // # singular name to be used as an alias on the CLI and for display
          singular: "storage",
          // kind is normally the CamelCased singular type. 
          // Your resource manifests use this.
          kind: "Storage",
          # shortNames allow shorter string to match your resource on the CLI
          shortNames: ["stor"],
        },
        additionalPrinterColumns: [
          {
            JSONPath: ".spec.capacity",
            name: "Capacity",
            description: "Capacity of the storage",
            type: "string",
          },
          {
            JSONPath: ".spec.nodeName",
            name: "NodeName",
            description: "Node where the storage gets attached",
            type: "string",
          },
          {
            JSONPath: ".status.phase",
            name: "Status",
            description: "Identifies the current status of the storage",
            type: "string",
          },
        ],
      }
    }
  ]
}
