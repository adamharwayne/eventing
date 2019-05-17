# Step-by-step: Deploy "Hello World" Eventing soluction

This step-by-step guide will guide you through all the steps involved to 
run “Hello World” eventing solution, where you will send an event to the 
Knative eventing broker inside your K8s namespace and receive it in a pod 
using Knative eventing. ETA - 20-30 mins. If you want to skip all the steps 
and want to quickly deploy the solution then please refer to 
[Quick Start: Deploy "Hello World" eventing solution](todo).

## Before you begin

Before you begin:
- Have a Kubernetes Cluster running version 1.12.
- [Install Knative Eventing](todo) and verify the health of the Knative Eventing
  control plane.
  
## Description

This sample is identical to [Quick Start: Deploy "Hello World" eventing solution](todo),
but rather than using a single YAML, we will apply each piece one step at a time, 
explaining what it does as we do so.

We will create the following topology:

Image from 2.

- [`Pod` named `curl`](#event-producer), acting as the event producer.
- `Broker` named `default`.
- Two [`Trigger`s](#triggers), each with a mutually exclusivefilter pointing at distinct 
  subscribers.
- Two Kubernetes [`Service`s](#event-consumers), each pointing at a unique:
    - [`Deployment`](#event-consumers), acting as the event consumer.

## Helpers

First we will create useful shell variables to allow us to not have to type 
things out every time.

```shell
K_NAMESPACE=kn-eventing-step-by-step-sample
```

## Namespace

Create the namespace.

```shell
kubectl create namespace $K_NAMESPACE
```

Now that the namespace has been created, we will label it to get Knative eventing
setup. Adding this label will:
- Create `ServiceAccount`s.
- Create `RoleBinding`s to give permissions to those `ServiceAccount`s.
- Create a `Broker` named `default`.

```shell
kubectl label namespace $K_NAMESPACE knative-eventing-injection=enabled
```

### Broker

What is a `Broker`? It can be thought of as an event mesh. Event
producers send events into the `Broker`. Event consumers register the kinds of events
they are interested in with the `Broker` (via `Trigger`s, described later). The
`Broker` ensures that every event sent to the `Broker` by event producers is sent
to all interested event consumers.


Let's verify that the `Broker` is in a healthy state.

```shell
kubectl -n $K_NAMESPACE get broker default
```

This should return something like:

```shell
NAME      READY   REASON   HOSTNAME                                                           AGE
default   True             default-broker.kn-eventing-step-by-step-sample.svc.cluster.local   1m
```

We are particularly interested in the second column, `READY`. We want that value to
be `True`. It may take a minute or two to become `True`. If it is not `True` within
two minutes, then something has gone wrong. Hopefully the `REASON` column gives a
useful error message. See our [Broker Debugging Guide](somewhere) to help fix the
error.

## Event Consumers

The first thing we are going to do is create our event consumers. These are 
`Deployment`s, and `Service`s that point at them. We are creating two distinct
event consumers, so that we can show how to selectively send events to distinct
event consumers later.

Setup the '-dev' `Service` and `Deployment`.

```shell
cat << END | kubectl -n $K_NAMESPACE apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: event-display-dev
spec:
  replicas: 1
  selector:
    matchLabels: &labels
      app: event-display-dev
  template:
    metadata:
      labels: *labels
    spec:
      containers:
        - name: event-display
          # Source code: https://github.com/knative/eventing-sources/blob/release-0.6/cmd/event_display/main.go
          image: gcr.io/knative-releases/github.com/knative/eventing-sources/cmd/event_display@sha256:37ace92b63fc516ad4c8331b6b3b2d84e4ab2d8ba898e387c0b6f68f0e3081c4

---

# Service pointing at the previous Deployment. This will be the target for event
# consumption.
kind: Service
apiVersion: v1
metadata:
  name: event-display-dev
spec:
  selector:
    app: event-display-dev
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
END
```

Now setup the '-test' `Service` and `Deployment`:

```shell
cat << END | kubectl -n $K_NAMESPACE apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: event-display-test
spec:
  replicas: 1
  selector:
    matchLabels: &labels
      app: event-display-test
  template:
    metadata:
      labels: *labels
    spec:
      containers:
        - name: event-display
          # Source code: https://github.com/knative/eventing-sources/blob/release-0.6/cmd/event_display/main.go
          image: gcr.io/knative-releases/github.com/knative/eventing-sources/cmd/event_display@sha256:37ace92b63fc516ad4c8331b6b3b2d84e4ab2d8ba898e387c0b6f68f0e3081c4

---

# Service pointing at the previous Deployment. This will be the target for event
# consumption.
kind: Service
apiVersion: v1
metadata:
  name: event-display-test
spec:
  selector:
    app: event-display-test
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
END
```

Let's wait for the `Deployment`s to be ready, as shown by their Available status
condition.

```shell
kubectl -n $K_NAMESPACE get deployments event-display-dev event-display-test                                                                                                                                            +54 10:30 ❰─┘
```

This should return something like:
```shell
NAME                 DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
event-display-dev    1         1         1            1           16m
NAME                 DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
event-display-test   1         1         1            1           16m
```

In particular, we want the number in the `DESIRED` column to match the number in the
`AVAILABLE` column. This may take a few minutes. If after three minutes, it the
numbers still do not match, then investigate why. 

TODO Add an investigation guide.

## Triggers

Earlier we said that different event consumers can register their intereset with a
`Broker`. That interest is registered by `Trigger` objects. A `Trigger` object
logically breaks into two pieces:
- Filter - What events I am interested in.
- Subscriber - Where should those events be sent.

### Filter

The Filter can be seen in `spec.filter`. For example:
```yaml
spec:
  filter:
    sourceAndType:
      type: knative.eventing.hello_world
      source: https://knative.eventing.dev
```

Every valid CloudEvent has attributes named `Type` and `Source`. `Trigger`s allow
you to specify interest in specific CloudEvents by matching the CloudEvents `Type`
and `Source`.  

For example, if we want all CloudEvents of type `foo`, regardless of their `Source`,
then we could use the following filter.

```yaml
spec:
  filter:
    sourceAndType:
      type: foo
```

Or if we want all CloudEvents with source `bar`, regardless of their `Type`, then
we could use the following filter:

```yaml
spec:
  filter:
    sourceAndType:
      source: bar
```

### Subscriber

Subscriber describes where to send CloudEvents. Once the filter has matched a
CloudEvent, then the Subscriber describes where it is sent. Subscriber is an
Object Reference to an `Addressable`. Normally it is a `Broker`, `Channel`, 
K8s `Service`, or Knative `Service`.

### Creation

We will now create one `Trigger` each for the `-dev` and `-test` event display
`Service`s we made in the previous step.

This `Trigger` will send events to the `-dev` `Service`.
```shell
cat << END | kubectl -n $K_NAMESPACE apply -f -
apiVersion: eventing.knative.dev/v1alpha1
kind: Trigger
metadata:
  name: event-display-dev
spec:
  filter:
    sourceAndType:
      type: knative.eventing.hello_world
      source: https://knative.eventing.dev
  subscriber:
    ref:
     apiVersion: v1
     kind: Service
     name: event-display-dev
END
```

This `Trigger` will send events to the `-test` `Service`. 

```shell
cat << END | kubectl -n $K_NAMESPACE apply -f -
apiVersion: eventing.knative.dev/v1alpha1
kind: Trigger
metadata:
  name: event-display-test
spec:
  filter:
    sourceAndType:
      type: knative.eventing.hello_world
      source: https://knative.eventing.test
  subscriber:
    ref:
     apiVersion: v1
     kind: Service
     name: event-display-test
END
```


**Note** that the filters are different. Both only allow events with type 
`knative.eventing.hello_world`, but the `-dev` `Trigger` requires the source to be
`https://knative.eventing.dev`, while the `-test` `Trigger` requires the source to
be `https://knative.eventing.test`.

### Verification

Let's verify that our `Trigger`s are running correctly.

```shell
kubectl -n $K_NAMESPACE get triggers
```

We expect to see something like:

```shell
NAME                 READY   REASON   BROKER    SUBSCRIBER_URI                                                                 AGE
event-display-dev    True             default   http://event-display-dev.kn-eventing-step-by-step-sample.svc.cluster.local/    16s
event-display-test   True             default   http://event-display-test.kn-eventing-step-by-step-sample.svc.cluster.local/   9s
```

The important column is the `READY` column. We want the value to be `True` for all
`Trigger`s. It may take one minute for the `Trigger`s  to reach `True`. If it isn't
`True` after a minute, something has gone wrong. See [Trigger debugging guide](todo).

## Recap

So far, we have created a namespace and had a `Broker` created in it. Then we 
created a pair of event consumers and registered their interest in a certain
events by creating `Trigger`s.

## Event Producer

We will be creating a CloudEvent by sending an HTTP request to directly to the
`Broker`. There are libraries that make this easy, such as 
[CloudEvents Go SDK](todo), but for simplicity we will craft a curl request
manually.

First, let's verify the hostname of the `Broker`. 

```shell
kubectl -n $K_NAMESPACE get broker default -o jsonpath='{.status.address.hostname}'
```

This should be `default-broker.kn-eventing-step-by-step-sample.svc.cluster.local`.
The trailing `.cluster.local` may be different based on how your Kubernetes cluster
is setup. Use the hostname as the host for all the curl commands that follow (note
that if your hostname is different, then you will need to replace it in the curl
commands).

The `Broker` is only exposed from within the Kubernetes cluster, so we will create
a `Pod`, SSH into that `Pod`, and run the curl request from there.

Create the `Pod`:

```shell
cat << END | kubectl -n $K_NAMESPACE apply -f -
apiVersion: v1
kind: Pod
metadata:
  labels:
    run: curl
  name: curl
spec:
  containers:
    # This could be any image that we can SSH into and has curl.
  - image: radial/busyboxplus:curl
    imagePullPolicy: IfNotPresent
    name: curl
    resources: {}
    stdin: true
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    tty: true
END
```

### SSH

Once the `Pod` is running, SSH into the `Pod`:

```shell
kubectl -n $K_NAMESPACE attach curl -it
```

From within that SSH terminal, we will make three curl requests:
- Request that goes to `-dev`.
- Request that goes to `-test`.
- Request that goes to neither.

#### Request to `-dev`

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-dev" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: knative.eventing.hello_world" \
  -H "Ce-Source: https://knative.eventing.dev" \
  -H "Content-Type: application/json" \
  -d '{"msg":"Hello World event from Knative - Dev"}'
```

We should receive a `202 Accepted` response.

#### Request to `-test`

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-test" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: knative.eventing.hello_world" \
  -H "Ce-Source: https://knative.eventing.test" \
  -H "Content-Type: application/json" \
  -d '{"msg":"Hello World event from Knative - Test"}'
```

We should receive a `202 Accepted` response.

#### Request to neither

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-neither" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: knative.eventing.hello_world" \
  -H "Ce-Source: https://knative.eventing.something-else" \
  -H "Content-Type: application/json" \
  -d '{"msg":"Hello World event from Knative - Neither"}'
```

We should receive a `202 Accepted` response.

## End to end Verification

Now that we have sent the events, let's verify that they were received by the
appropriate subscribers.

First, exit out of any SSH terminals. All the remaining commands will take place
on the machine that has been running `kubectl`.

We are going to check the logs of each event consumer to verify it saw only the
event that is expected.

### `-dev`

Use the following command to see the logs for the `-dev` event consumer.

```shell
kubectl -n $K_NAMESPACE logs -l app=event-display-dev
```

This should output something like:

```shell
☁️   cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 0.2
  type: knative.eventing.hello_world
  source: https://knative.eventing.dev
  id: should-be-seen-by-dev
  time: 2019-05-17T18:50:14.003025764Z
  contenttype: application/json
Extensions,
  knativehistory: default-broker-krv99-channel-j9fcg.kn-eventing-step-by-step-sample.svc.cluster.local
Data,
  {
    "msg": "Hello World event from Knative - Dev"
  }
```

**Note**: There should be exactly one event seen in the output, because of the
three events we sent, only one matched the `-dev` filter, so we expect only that
one to be seen. The single event seen should have a source of 
`https://knative.eventing.dev`.

If the output does not look as expected, then something has gone wrong. Look at 
our [Broker Debugging Guide](todo) to help debug the issue.


### `-test`

Use the following command to see the logs for the `-test` event consumer.

```shell
kubectl -n $K_NAMESPACE logs -l app=event-display-test
```

This should output something like:

```shell
☁️   cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 0.2
  type: knative.eventing.hello_world
  source: https://knative.eventing.test
  id: should-be-seen-by-test
  time: 2019-05-17T18:50:50.447806265Z
  contenttype: application/json
Extensions,
  knativehistory: default-broker-krv99-channel-j9fcg.kn-eventing-step-by-step-sample.svc.cluster.local
Data,
  {
    "msg": "Hello World event from Knative - Test"
  }
```

**Note**: There should be exactly one event seen in the output, because of the
three events we sent, only one matched the `-dev` filter, so we expect only that
one to be seen. The single event seen should have a source of 
`https://knative.eventing.test`.

If the output does not look as expected, then something has gone wrong. Look at 
our [Broker Debugging Guide](todo) to help debug the issue.

## Cleanup

Clean up is easy, just delete the namespace.

```shell
kubectl delete namespace $K_NAMESPACE
```

## Next Steps

...
