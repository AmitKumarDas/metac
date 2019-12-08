# Metac `pronounced [meta-see]`
It is [metacontroller](https://github.com/GoogleCloudPlatform/metacontroller) and more. Long term vision of Metac is to provide a toolkit that lets users to manage their infrastructures on Kubernetes.

Metac started when development on metacontroller stopped. Metac has implemented most of the major enhancements & issues raised in [metacontroller](https://github.com/GoogleCloudPlatform/metacontroller/issues). In adition, some of Metac's features are a derivation from production needs of projects such as [OpenEBS](https://github.com/openebs) & [LitmusChaos](https://github.com/litmuschaos).

## Motivation
Metac is an add-on for Kubernetes that makes it easy to write and deploy [custom controllers](https://kubernetes.io/docs/concepts/api-extension/custom-resources/#custom-controllers) in the form of [simple scripts](https://metacontroller.app). One can get a feel of implementing controllers from various sample implementations found in the examples folder. These examples showcase various approaches, programming languages (including jsonnet) to implement controllers. 

## Features
These are some the features that metac supports:
- Abstracts Kubernetes code from business logic
- Implements various meta controllers that helps in above abstraction
    - CompositeController (cluster scoped)
    - DecoratorController (cluster scoped)
    - GenericController (namespace scoped)
- Business logic _(read reconciliation logic)_ can be exposed as http services
    - API based development as first class citizen
- MetaControllers are deployed as Kubernetes custom resources
    - However, GenericController _(one of the meta controllers)_ can either be deployed as:
        - 1/ Kubernetes based custom resources, or
        - 2/ YAML config file.
- Ability to import metac as a go library
    - GenericController lets business logic invoked as in-line function call(s)
    - This is an additional way to invoke logic other than http calls
    - Hence, no need to write reconcile logic as http services if not desired

## Using Metac
If you want to use Metac via web based hooks then Metac can be deployed as a StatefulSet with images found at this [registry](https://quay.io/repository/amitkumardas/metac?tab=tags). However, if you want to use inline hooks, you need to import Metac into your go based controller implementation. In addition, you need to make use of go modules to import the master version of Metac into your codebase.

In case, you want to deploy Metac via `helm`, use this [helm chart](https://github.com/AmitKumarDas/metac/tree/master/helm/metac).

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

## Roadmap
These are the broad areas of focus for metac:
- [x] business controllers
- [ ] test controllers
- [ ] debug controllers
- [ ] compliance controllers

## Documentation

This is the existing i.e. [metacontroller site](https://metacontroller.app) that provides most of the important details about Metacontroller. Since metac does not differ from Metacontroller except for new enhancements and fixes, this doc site holds good.

## Contact

Please file [GitHub issues](issues) for bugs, feature requests, and proposals.

Use the [meeting notes/agenda](https://docs.google.com/document/d/1HV_Fr0wIW9tr5OZwK_6oGux_OhcGtxxWWV6dCYJR9Cw/) to discuss specific features/topics with the community.

Join [#metacontroller](https://kubernetes.slack.com/messages/metacontroller/) channel on
[Kubernetes Slack](http://slack.kubernetes.io).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [contributor guide](https://metacontroller.app/contrib/).

## Licensing

This project is licensed under the [Apache License 2.0](LICENSE).

## Comparison with other operators
Among most of the articles found in internet, I find [this](https://admiralty.io/blog/kubernetes-custom-resource-controller-and-operator-development-tools/) to be really informative. However, it talks about metacontroller whereas metac has filled in most of the gaps left by the former.
