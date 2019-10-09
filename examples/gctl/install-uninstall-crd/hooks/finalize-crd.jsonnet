function(request) {
  // Delete the CRD since namespace is observed 
  // to being deleted
  attachments: [],
  // Mark as finalized once we observe this CRD are gone.
  finalized: std.length(request.attachments['CustomResourceDefinition.apiextensions.k8s.io/v1beta1']) == 0
}
