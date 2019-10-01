---
title: Troubleshooting
---
This is a collection of tips for debugging controllers written with Metacontroller.

If you have something to add to the collection, please send a pull request against
[this document]({{ site.repo_file }}/docs/guide/troubleshooting.md).

## Metacontroller Logs

Until Metacontroller [emits events]({{ site.repo_url }}//issues/7),
the first place to look when troubleshooting controller behavior is the logs for
the Metacontroller server itself.

For example, you can fetch the last 25 lines with a command like this:

```sh
kubectl -n metacontroller logs --tail=25 -l app=metacontroller
```

### Log Levels

You can customize the verbosity of the Metacontroller server's logs with the
`-v=N` flag, where `N` is the log level.

At all log levels, Metacontroller will log the progress of server startup and
shutdown, as well as major changes like starting and stopping hosted controllers.

Metacontroller will log the following events based on log levels:
- level 1: startup, shutdown, error, create & delete actions
- level 2: update actions
- level 3: ignore/skip actions due to specifics
- level 4: creating, updating, deleting, debugging, ignore/skip actions due to defaults
- level 5: diff of objects
- level 6: hook invocation along with JSON req & response bodies

### Common Log Messages

#### Errors due to new CRDs
Since API discovery info is refreshed periodically, you may see log messages
like this when you start a controller that depends on a recently-installed CRD:

```
failed to sync CompositeController "my-controller": discovery: can't find resource <resource> in apiVersion <group>/<version>
```

Usually, this should fix itself within about 30s when the new CRD is discovered.
If this message continues indefinitely, check that the resource name and API
group/version are correct.

#### Messages due to periodic flush
You may also notice periodic log messages like this:

```
Watch close - *unstructured.Unstructured total <X> items received
```

This comes from the underlying client-go library, and just indicates when the
shared caches are periodically flushed to place an upper bound on cache
inconsistency due to potential silent failures in long-running watches.

#### Errors connecting to WebHook service

```
E1001 06:41:20.115843       1 controller.go:330] failed to sync service-per-pod "apps/v1:StatefulSet:default:nginx": Sync hook failed: Webhook url=http://jsonnetd.metac/sync-service-per-pod timeout=10000000000: Invoke failed: Post http://jsonnetd.metac/sync-service-per-pod: dial tcp: lookup jsonnetd.metac on 127.0.0.53:53: read udp 127.0.0.1:38446->127.0.0.53:53: read: connection refused
```
- Check the services of your Kubernetes cluster
- I got above error while trying with micro.kubectl

```
E1001 07:21:35.763117       1 controller.go:330] failed to sync pod-name-label "v1:Pod:default:nginx-2": can't update Pod default/nginx-2: Operation cannot be fulfilled on pods "nginx-2": the object has been modified; please apply your changes to the latest version and try again
```
- <To fill up>


## Webhook Logs

If you return an HTTP error code (e.g. 500) from your webhook,
the Metacontroller server will log the text of the response body.

If you need more detail on what's happening inside your hook code, as opposed to
what Metacontroller does for you, you'll need to add log statements to your own
code and inspect the logs on your webhook server.
