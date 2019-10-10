# Metac (pronounced [meta-see])

 [metac](https://github.com/AmitKumarDas/metac) is an add-on for Kubernetes that makes it easy to write and deploy custom controllers.

 It is metacontroller and more. This has additional custom resources derived from production needs of projects like OpenEBS & LitmusChaos. It will also package itself to be imported as a library by other Kubernetes operator based projects.

 While custom resources provide storage for new types of objects, custom controllers define the behavior of a new extension to the Kubernetes API. Just like the CustomResourceDefinition (CRD) API makes it easy to request storage for a custom resource, the Metacontroller APIs make it easy to define behavior for a new extension API or add custom behavior to existing APIs. (from Metacontroller [Website](https://metacontroller.app/))

 ## TL;DR;

 ```console
$ helm install stable/metac
```

 ## Introduction

 This chart bootstraps a metac on a [Kubernetes](http://kubernetes.io) cluster using the [Helm](https://helm.sh) package manager. This enables you to deploy custom controllers as service + deployment pairs (see [docs](https://metac.app/guide/create/))

 ## Installing the Chart

 To install the chart with the release name `my-release`:

 ```console
$ helm install stable/metac --name my-release
```

 The command deploys metac on the Kubernetes cluster in the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation. There is a high likelihood that you will want the default configuration.

 ## Uninstalling the Chart

 To uninstall/delete the `my-release` deployment:

 ```console
$ helm delete my-release
```
or

 ```console
$ helm delete --purge my-release
```

 The command removes all the Kubernetes components associated with the chart and deletes the release.

 ## Configuration

 The following table lists the configurable parameters of the metac chart and their default values.

 Parameter | Description | Default
--- | --- | ---
`image.repository` | `metac` image repository  | `quay.io/amitkumardas/metac`
`image.tag` | `metac` image tag  | `latest`
`rbac.create` | Specifies whether RBAC resources should be created | `true`
`logLevel` | Specify the log level, should be a value between 1 and 4 | `1`
`discoveryInterval` | How often to refresh discovery cache to pick up newly-installed resource | `10s`
`cacheFlushInterval` | How often to flush local caches and relist objects from the API server | `24h`
`workerCount` | How many workers to start per controller to process queued events | `5`
`clientGoQps` | Number of queries per second client-go is allowed to make | `5`
`clientGoBurst` | Allowed burst queries for client-go | `10`

 Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example,

 ```console
$ helm install stable/metac --name my-release \
  --set=image.tag=0.2,resources.limits.cpu=200m
```

 Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

 ```console
$ helm install stable/metac --name my-release -f values.yaml
```

 > **Tip**: You can use the default [values.yaml](values.yaml)
 ## Notes
You can only install one copy of this chart in a given Kubernetes cluster, due to the fact that it has CRDs included in it. If you attempt to install a second copy, you'll get an error like so:

 ```console
Error: release my-release failed: customresourcedefinitions.apiextensions.k8s.io "compositecontrollers.metac.openebs.io" already exists
```

You can set `crds.create` to `false` to prevent this.
