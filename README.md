# Metac `pronounced [meta-see]`
It is [metacontroller](https://github.com/GoogleCloudPlatform/metacontroller) and more. Metac has additional features derived from various production needs of projects such as [OpenEBS](https://github.com/openebs) & [LitmusChaos](https://github.com/litmuschaos). In addition, metac solves most of the user issues raised in [metacontroller](https://github.com/GoogleCloudPlatform/metacontroller/issues).

## Motivation
Metacontroller is an add-on for Kubernetes that makes it easy to write and deploy [custom controllers](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#custom-controllers) in the form of [simple scripts](https://metacontroller.app).

## Features
These are some the features that metac supports:
- Abstracts Kubernetes code from business logic
- Implements various MetaControllers that helps in above abstraction
    - CompositeController (cluster scoped)
    - DecoratorController (cluster scoped)
    - GenericController (namespace scoped)
- Business logic _(read reconciliation logic)_ can be exposed as http services
    - API based development as first class citizen
- MetaControllers are defined as Kubernetes CustomResourceDefinitions
- GenericController _(one of the meta controllers)_ can be either deployed as:
    - Kubernetes CRD, or
    - YAML config to metac binary
- GenericController lets business logic invoked as in-line function call(s)
    - This is an additional way to invoke logic other than http calls
    - Hence, no need to write reconcile logic as http services if not desired

## Differences from metacontroller
Metac tries to be compatible with the original metacontroller. However, there may be breaking changes that one needs to be careful about. If one has been using the metacontroller and tries to use metac, then one should be aware of below changes:
- Metac uses a different api group for the custom resources
    - i.e. `apiVersion: metac.openebs.io/v1alpha1`
- Metac uses a different set of finalizers
   - i.e. `metac.openebs.io/<controller-name>`
- Metac is by default installed in `metac` namespace

If you are migrating from Metacontroller to Metac you'll need to cleanup the old Metacontroller's finalizers, you can use a command like the following:

```console
kubectl get <comma separated list of your resource types here> --no-headers --all-namespaces | awk '{print $2 " -n " $1}' | xargs -L1 -P 50 -r kubectl patch -p '{"metadata":{"finalizers": [null]}}' --type=merge
```

## Documentation

Please see the [documentation site](https://metacontroller.app) for details on how to install, use, or contribute to Metacontroller.

## Contact

Please file [GitHub issues](issues) for bugs, feature requests, and proposals.

Use the [meeting notes/agenda](https://docs.google.com/document/d/1HV_Fr0wIW9tr5OZwK_6oGux_OhcGtxxWWV6dCYJR9Cw/) to discuss specific features/topics with the community.

Use the [mailing list](https://groups.google.com/forum/#!forum/metacontroller)
for questions and comments, or join the
[#metacontroller](https://kubernetes.slack.com/messages/metacontroller/) channel on
[Kubernetes Slack](http://slack.kubernetes.io).

Subscribe to the [announce list](https://groups.google.com/forum/#!forum/metacontroller-announce)
for low-frequency project updates like new releases.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [contributor guide](https://metacontroller.app/contrib/).

## Licensing

This project is licensed under the [Apache License 2.0](LICENSE).

## Comparison with other operators
Among most of the articles found in internet, I find [this](https://admiralty.io/blog/kubernetes-custom-resource-controller-and-operator-development-tools/) to be really informative. However, it talks about metacontroller whereas metac has filled in most of the gaps left by the former.