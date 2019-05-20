# Step-by-step: Deploy "Hello World" Eventing soluction

This step-by-step guide will guide you through all the steps involved to 
run “Hello World” eventing solution, where you will send an event to the 
Knative eventing broker inside your K8s namespace and receive it in a pod 
using Knative eventing. ETA - 20-30 mins. If you want to skip all the steps 
and want to quickly deploy the solution then please refer to 
[Quick Start: Deploy "Hello World" eventing solution](todo).

## Before you begin

Before you begin:
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
- Two [`Trigger`s](#triggers), each with a distinct filter pointing at distinct 
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
they are interested in with the `Broker` (via `Trigger`s, 
[described later](#triggers)). The
`Broker` ensures that every event sent to the `Broker` by event producers is sent
to all interested event consumers. To learn more about `Broker`s, see [`Broker` in
Eventing Concepts](todo).


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
useful error message. See our [Broker Debugging Guide](todo) to help fix the
error.

## Event Consumers

The first thing we are going to do is create our event consumers. These are 
`Deployment`s, and `Service`s that point at them. We are creating two distinct
event consumers, so that we can show how to selectively send events to distinct
event consumers later.

Setup the 'foo' `Service` and `Deployment`.

**TODO** Use a Google Cloud style tabbed interface to allow people to choose if they
wnat bash, or download a yaml and run, something else... Similar to Google Cloud
pages that show gcloud, Cloud Console, and API tabs for their examples.


```shell
kubectl -n $K_NAMESPACE apply -f - << END
apiVersion: apps/v1
kind: Deployment
metadata:
  name: foo-display
spec:
  replicas: 1
  selector:
    matchLabels: &labels
      app: foo-display
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
  name: foo-display
spec:
  selector:
    app: foo-display
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
END
```

Now setup the 'bar' `Service` and `Deployment`:

```shell
kubectl -n $K_NAMESPACE apply -f - << END
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bar-display
spec:
  replicas: 1
  selector:
    matchLabels: &labels
      app: bar-display
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
  name: bar-display
spec:
  selector:
    app: bar-display
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080
END
```

Let's wait for the `Deployment`s to be ready, as shown by their Available status
condition.

```shell
kubectl -n $K_NAMESPACE get deployments foo-display bar-display
```

This should return something like:
```shell
NAME           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
foo-display    1         1         1            1           16m
NAME           DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
bar-display    1         1         1            1           16m
```

In particular, we want the number in the `DESIRED` column to match the number in the
`AVAILABLE` column. This may take a few minutes. If after three minutes, it the
numbers still do not match, then investigate why. 

TODO Add an investigation guide.

## Triggers

Earlier we said that different event consumers can register their interest with a
`Broker`. That interest is registered by `Trigger` objects. A `Trigger` object
logically breaks into two pieces:
- Filter - What events I am interested in.
- Subscriber - Where should those events be sent.

To learn more about `Broker`s, see [`Trigger` in Eventing Concepts](todo).

### Filter

The Filter can be seen in `spec.filter`. For example:
```yaml
spec:
  filter:
    sourceAndType:
      type: foo
      source: bar
```

Every valid [CloudEvent](https://github.com/cloudevents/spec/blob/v0.2/spec.md)
has attributes named `Type` and `Source`. `Trigger`s allow
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
Object Reference to an [`Addressable`](addressable-concept) object. Normally it 
is a [`Broker`](todo), [`Channel`](todo), 
[Kubernetes `Service`](https://kubernetes.io/docs/concepts/services-networking/service/),
 or [Knative `Service`](todo).

### Creation

We will now create one `Trigger` each for the `foo` and `bar` event display
`Service`s we made in the previous step.

This `Trigger` will send events to the `foo` `Service`.
```shell
kubectl -n $K_NAMESPACE apply -f - << END
apiVersion: eventing.knative.dev/v1alpha1
kind: Trigger
metadata:
  name: foo-display
spec:
  filter:
    sourceAndType:
      type: foo
  subscriber:
    ref:
     apiVersion: v1
     kind: Service
     name: foo-display
END
```

This `Trigger` will send events to the `bar` `Service`. 

```shell
kubectl -n $K_NAMESPACE apply -f - << END
apiVersion: eventing.knative.dev/v1alpha1
kind: Trigger
metadata:
  name: bar-display
spec:
  filter:
    sourceAndType:
      source: bar
  subscriber:
    ref:
     apiVersion: v1
     kind: Service
     name: bar-display
END
```


**Note** that the filters are different. One only checks the Cloud Event's `type`, the other only checks the Cloud Event's `source`.

### Verification

Let's verify that our `Trigger`s are running correctly.

```shell
kubectl -n $K_NAMESPACE get triggers
```

We expect to see something like:

```shell
NAME                 READY   REASON   BROKER    SUBSCRIBER_URI                                                                 AGE
bar-display          True             default   http://bar-display.kn-eventing-step-by-step-sample.svc.cluster.local/   9s
foo-display          True             default   http://foo-display.kn-eventing-step-by-step-sample.svc.cluster.local/    16s
```

The important column is the `READY` column. We want the value to be `True` for all
`Trigger`s. It may take one minute for the `Trigger`s  to reach `True`. If it isn't
`True` after a minute, something has gone wrong. See [Trigger debugging guide](todo).

## Recap

So far, we have created a namespace and had a `Broker` created inside it. Then we 
created a pair of event consumers and registered their interest in a certain
events by creating `Trigger`s.

## Event Producer

We will create a CloudEvent by sending an HTTP request directly to the
`Broker`. There are libraries that make this easy, such as 
[CloudEvents Go SDK](https://github.com/cloudevents/sdk-go), but for simplicity we will craft a curl request
manually.

First, let's verify the hostname of the `Broker`. 

```shell
kubectl -n $K_NAMESPACE get broker default -o jsonpath='{.status.address.hostname}'
```

This should be `default-broker.kn-eventing-step-by-step-sample.svc.cluster.local`.
The trailing `.cluster.local` may be different based on how your Kubernetes cluster
is setup. Use the hostname as the host for all the curl commands that follow (**note**
that if your hostname is different, then you will need to replace it in the curl
commands).

The `Broker` is only exposed from within the Kubernetes cluster, so we will create
a `Pod`, SSH into that `Pod`, and run the curl request from there.

Create the `Pod`:

```shell
kubectl -n $K_NAMESPACE apply -f - << END
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

From within that SSH terminal, we will make four curl requests:
- [Request that goes exclusively to `foo`](#request-to-foo).
- [Request that goes exclusively to `bar`](#request-to-bar).
- [Request that goes to both `foo` and `bar`](#request-to-both).
- [Request that goes to neither `foo` nor `bar`](#request-to-neither).

#### Request to `foo`

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-foo" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: foo" \
  -H "Ce-Source: anything-but-bar" \
  -H "Content-Type: application/json" \
  -d '{"msg":"Hello World event from Knative - Foo"}'
```

We should receive a `202 Accepted` response.

#### Request to `bar`

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-test" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: anything-but-foo" \
  -H "Ce-Source: bar" \
  -H "Content-Type: application/json" \
  -d '{"msg":"Hello World event from Knative - Bar"}'
```

We should receive a `202 Accepted` response.

#### Request to both

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-test" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: foo" \
  -H "Ce-Source: bar" \
  -H "Content-Type: application/json" \
  -d '{"msg":"Hello World event from Knative - Both"}'
```

We should receive a `202 Accepted` response.

#### Request to neither

From within the SSH terminal:

```shell
 curl -v "default-broker.kn-eventing-step-by-step-sample.svc.cluster.local" \
  -X POST \
  -H "Ce-Id: should-be-seen-by-neither" \
  -H "Ce-Specversion: 0.2" \
  -H "Ce-Type: anything-but-foo" \
  -H "Ce-Source: anything-but-bar" \
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
events that are expected.

### `foo` Verification

Use the following command to see the logs for the `foo` event consumer.

```shell
kubectl -n $K_NAMESPACE logs -l app=foo-display
```

This should output something like:

```shell
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 0.2
  type: foo
  source: anything-but-bar
  id: should-be-seen-by-foo
  time: 2019-05-20T17:59:43.81718488Z
  contenttype: application/json
Extensions,
  knativehistory: default-broker-srk54-channel-24gls.kn-eventing-step-by-step-sample.svc.cluster.local
Data,
  {
    "msg": "Hello World event from Knative - Foo"
  }
☁️  cloudevents.Event
Validation: valid
Context Attributes,
  specversion: 0.2
  type: foo
  source: bar
  id: should-be-seen-by-test
  time: 2019-05-20T17:59:54.211866425Z
  contenttype: application/json
Extensions,
  knativehistory: default-broker-srk54-channel-24gls.kn-eventing-step-by-step-sample.svc.cluster.local
Data,
  {
    "msg": "Hello World event from Knative - Both"
  }
```

**Note**: There should be exactly two events seen in the output, because of the
four events we sent, only two matched the `foo` filter, so we expect only those to be seen.

If the output does not look as expected, then something has gone wrong. Look at 
our [Broker Debugging Guide](todo) to help debug the issue.


### `bar` Verification

Use the following command to see the logs for the `bar` event consumer.

```shell
kubectl -n $K_NAMESPACE logs -l app=bar-display
```

This should output something like:

```shell
☁️  cloudevents.Event
 Validation: valid
 Context Attributes,
   specversion: 0.2
   type: anything-but-foo
   source: bar
   id: should-be-seen-by-test
   time: 2019-05-20T17:59:49.044926148Z
   contenttype: application/json
 Extensions,
   knativehistory: default-broker-srk54-channel-24gls.kn-eventing-step-by-step-sample.svc.cluster.local
 Data,
   {
     "msg": "Hello World event from Knative - Bar"
   }
 ☁️  cloudevents.Event
 Validation: valid
 Context Attributes,
   specversion: 0.2
   type: foo
   source: bar
   id: should-be-seen-by-test
   time: 2019-05-20T17:59:54.211866425Z
   contenttype: application/json
 Extensions,
   knativehistory: default-broker-srk54-channel-24gls.kn-eventing-step-by-step-sample.svc.cluster.local
 Data,
   {
     "msg": "Hello World event from Knative - Both"
   } 
```

**Note**: There should be exactly two events seen in the output, because of the
four events we sent, only two matched the `foo` filter, so we expect only those to be seen.

If the output does not look as expected, then something has gone wrong. Look at 
our [Broker Debugging Guide](todo) to help debug the issue.

## Cleanup

Clean up is easy, just delete the namespace.

```shell
kubectl delete namespace $K_NAMESPACE
```

## Next Steps

- [Using event importer to consume events](todo)
- [Configure Broker to use a different Channel](todo)
- [Introduction to Event Registry](todo)

## Further Reading

- [Eventing Concepts](todo) - Descriptions of all the Knative Eventing pieces and
  how the interact.
- [Advanced filtering using Triggers](todo)
