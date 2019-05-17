# Configure `Broker` to use a different `Channel`

`Broker`'s compose multiple infrastructure pieces to provide a useful abstraction.
The most important infrastructure piece they use is `Channel`s. The specific 
`Channel` implementation used will dictate many properties of the `Broker`, including:
- Event durability.
- Event delivery retries.
- Event delivery ordering. 

As well as properties like event delivery latency.

To learn more about `Broker`s, see [`Broker` in Eventing Concepts](todo).

## Helpers

First we will create useful shell variables to allow us to not have to type things
out every time. Feel free to customize these to whatever is appropriate for your
`Broker`.

```shell
# The namespace the Broker is in.
BROKER_NS=kn-eventing-sample

# The name of the Broker.
BROKER_NAME=default
```

## Which `Channel` is my `Broker` _theoretically_ using?

To determine which `Channel` a `Broker` should _theoretically_ use, we look in two
places.

### Broker's `spec.channelTemplate`

The `Broker`'s `spec.channelTemplate` takes precedence over all other settings. If
specified, its value is used and all other settings are ignored.

```shell
kubectl -n $BROKER_NS get broker $BROKER_NAME -o jsonpath='{.spec.channelTemplate}'
```

If the result looks like:

```shell
map[provisioner:map[apiVersion:eventing.knative.dev/v1alpha1 kind:ClusterChannelProvisioner name:gcp-pubsub]]
```

Then something is specified and will be used regardless of other settings. In the
result above, we can see it says `name:gcp-pubsub`, so the `Broker` will use
`gcp-pubsub` provisioned `Channel`s.

If the result is the empty string, then `spec.channelTemplate` is empty and the
[Default Channel](#default-channel) is used.

### Default Channel

When a `Channel` is created with an empty `spec`, it is filled in by a defaulting
webhook. See [Default Channel](default-channel-docs) for a more thorough
description of how this works.

Read the default Channel configuration:

```shell
kubectl -n knative-eventing get configmap default-channel-webhook -o yaml
```

The output should look something like:

```yaml
apiVersion: v1
data:
  default-channel-config: |
    clusterdefault:
      apiversion: eventing.knative.dev/v1alpha1
      kind: ClusterChannelProvisioner
      name: in-memory
    namespacedefaults:
      some-namespace:
        apiversion: eventing.knative.dev/v1alpha1
        kind: ClusterChannelProvisioner
        name: some-other-provisioner
kind: ConfigMap
metadata:
  name: default-channel-webhook
  namespace: knative-eventing
```

We are interestedc in the `data` section, which contains a YAML string. There are
two sections:
- `clusterdefault`
- `namespacedefaults`

If the `namespacedefaults` section contains the namespace the `Broker` is in (i.e.
$BROKER_NS), then it will be used. If not, then the `clusterdcefault` will be used.

In our example, $BROKER_NS is not in the `namespacedefaults` section, so we would
expect the `Broker` to use the `clusterdefault`, namely `in-memory`. 

### Recap

By this point, we should know the _theoretical_ `Channel` provisioner the `Broker`
will use.

In order of precendence.
1. The `Broker`'s `spec.channelTemplate`.
1. The Default Channel for $BROKER_NS.
    1. The specified default for $BROKER_NS.
    1. The cluster wide default.

## Which `Channel` is my `Broker` _actually_ using?

why is there a potential difference between theory and reality? 

There is a difference between the two because changing the `Channel` provisioner
being used will mean that any in-flight events will be lost. So, the Knative
control plane _does not_ automatically change it for you. If you want to change
the `Channel` provisioner of an existing `Broker`, then you must follow the
instructions in [Changing the Channel](#changing-the-channel).

### Channels

Each `Broker` creates two `Channel`s. They have the label 
`eventing.knative.dev/broker`
with the value $BROKER_NAME (e.g. `eventing.knative.dev/broker: default`).

We will use the following command to get the provisioner they are using:

```shell
kubectl -n $BROKER_NS get channels -l "eventing.knative.dev/broker=$BROKER_NAME" -o jsonpath='{.items[*].spec.provisioner.name}'
```

The output should look something like:

```shell
in-memory in-memory
```

The expectation is that those two `Channel`s always have the same provisioner. If
they don't then someone probably got part of the way through 
[Changing the Channel](#changing-the-channel), without finishing it.

## Changing the Channel

### New Broker

The safe thing to do is to create a new `Broker` with `spec.channelTemplate`
specified as you desire. For example:

```shell
kubectl -n $BROKER_NS create -f - << END
apiVersion: eventing.knative.dev/v1alpha1
kind: Broker
metadata:
  name: $BROKER_NAME
spec:
  channelTemplate:
    provisioner:
      apiVersion: eventing.knative.dev/v1alpha1
      kind: ClusterChannelProvisioner
      name: gcp-pubsub
END
```

### Replace existing Broker

If you need to replace an existing `Broker`, delete it first, then re-create it
as above.

```shell
kubectl -n $BROKER_NS delete broker $BROKER_NAME
kubectl -n $BROKER_NS create -f - << END
apiVersion: eventing.knative.dev/v1alpha1
kind: Broker
metadata:
  name: $BROKER_NAME
spec:
  channelTemplate:
    provisioner:
      apiVersion: eventing.knative.dev/v1alpha1
      kind: ClusterChannelProvisioner
      name: gcp-pubsub
END
```

The one major catch with this approach is if you are manipulating the `Broker`
named `default` and the [namespace is annotated](todo) to ensure the `Broker` named
`default` exists. In all likelihood, your `create` command will not happen until
after the reconciler has created one already. So we can do either of the following:
- [Remove the annotation](#remove-the-annotation)
- [Change the Channel inflight](#change-the-channel-inflight)

#### Remove the annotation

We will remove the annotation, make our changes, then re-add the annotation.

```shell
# Disable the annotation.
kubectl label namespace $BROKER_NS --overwrite 'eventing.knative.dev/injection=off

# Delete the existing Broker.
kubectl -n $BROKER_NS delete broker $BROKER_NAME

# Create the new Broker.
kubectl -n $BROKER_NS create -f - << END
apiVersion: eventing.knative.dev/v1alpha1
kind: Broker
metadata:
  name: $BROKER_NAME
spec:
  channelTemplate:
    provisioner:
      apiVersion: eventing.knative.dev/v1alpha1
      kind: ClusterChannelProvisioner
      name: gcp-pubsub
END

# Enable the annotation.
kubectl label namespace $BROKER_NS --overwrite 'eventing.knative.dev/injection=enabled
```

#### Change the Channel Inflight

**Dangerous**

Forcing the `Channel`s of an exsiting `Broker` to change is **dangerous** it will
drop all events that are currently inflight, along with new events until the control
and data plane are healthy again.

If you want to do this, then the procedure is to first update the `Channel` that
the `Broker` will [theorectically use](#which-channel-is-my-broker-_theoretically_-using).
Most likely via setting its `spec.channelTemplate`.

```shell
kubectl -n $BROKER_NS apply -f - << END
apiVersion: eventing.knative.dev/v1alpha1
kind: Broker
metadata:
  name: $BROKER_NAME
spec:
  channelTemplate:
    provisioner:
      apiVersion: eventing.knative.dev/v1alpha1
      kind: ClusterChannelProvisioner
      name: gcp-pubsub
END
```

Then manually delete the existing `Channel`s:

```shell
kubectl -n $BROKER_NS delete channels -l "eventing.knative.dev/broker=$BROKER_NAME" 
```

That should delete the existing `Channel`s. Replacement `Channel`s will be
created very quickly by by the `Broker` reconciler.

```shell
kubectl -n $BROKER_NS get channels -l "eventing.knative.dev/broker=$BROKER_NAME"
```

Which will return something like:

```shell
NAME                   READY   REASON   AGE
default-broker-krv99   True             5s
default-broker-zjrjd   True             5s
```

Note the `AGE` column says they are very young (five seconds in this example).

Verify that they have the correct provisioner:

```shell
kubectl -n $BROKER_NS get channels -l "eventing.knative.dev/broker=$BROKER_NAME" -o jsonpath='{.items[*].spec.provisioner.name}'
```

The output should look something like:

```shell
gcp-pubsub gcp-pubsub
```

## Further Reading

- [Eventing Concepts](todo) - Descriptions of all the Knative Eventing pieces and
  how the interact.
