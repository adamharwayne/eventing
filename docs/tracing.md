## Tracing Demo

### Setup

1. Create a cluster on GKE.
1. Install Knative Eventing, likely at master.
    ```shell script
    ko apply -f config/
    ko apply -f config/channels/in-memory-channel/
    ```
1. Set the tracing to debug mode and Stackdriver.
    ```shell script
   kubectl patch configmap -n knative-eventing config-tracing -p '{"data":{"backend":"stackdriver","debug":"true","sample-rate":"1.0"}}' 
   ```
 
### Topology

1. Create a Broker.
    ```shell script
    kubectl label namespace default knative-eventing-injection=enabled 
    ```
1. Create a logging Pod and a K8s Service in front of it.
    ```shell script
    kubectl apply -f - << END
    apiVersion: v1
    kind: Service
    metadata:
      name: event-display
    spec:
      selector:
        app: event-display
      ports:
        - protocol: TCP
          port: 80
          targetPort: 8080
    
    ---
    
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: event-display
    spec:
      replicas: 1
      selector:
        matchLabels: &labels
          app: event-display
      template:
        metadata:
          labels: *labels
        spec:
          containers:
            - name: user-container
              image: gcr.io/knative-releases/github.com/knative/eventing-sources/cmd/event_display
              ports:
                - containerPort: 8080
    END
    ```
1. Create a Trigger pointing at that K8s Service.
    ```shell script
    kubectl apply -f - << END
    apiVersion: eventing.knative.dev/v1alpha1
    kind: Trigger
    metadata:
      name: display-mutated
    spec:
      filter:
        sourceAndType:
          type: mutated
      subscriber:
        ref:
         apiVersion: v1
         kind: Service
         name: event-display
    END
    ```
1. Create a mutating Pod and a K8s Service in front of it.
    ```shell script
    kubectl apply -f - << END
    apiVersion: v1
    kind: Service
    metadata:
      name: mutator-svc
    spec:
      selector:
        app: event-mutator
      ports:
        - protocol: TCP
          port: 80
          targetPort: 8080
    
    ---
    
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: event-mutator
    spec:
      replicas: 1
      selector:
        matchLabels: &labels
          app: event-mutator
      template:
        metadata:
          labels: *labels
        spec:
          containers:
            - name: user-container
              image: gcr.io/harwayne2/event_mutator-6675b31b36046f5bbbdbf976802c625f
              ports:
                - containerPort: 8080
    END
    ```
1. Create a Trigger pointing at that K8s Service.
    ```shell script
    kubectl apply -f - << END
    apiVersion: eventing.knative.dev/v1alpha1
    kind: Trigger
    metadata:
      name: mutator-t
    spec:
      filter:
        sourceAndType:
          type: com.example.someevent
      subscriber:
        ref:
         apiVersion: v1
         kind: Service
         name: mutator-svc
    END
 ``  

### Send an event

1. Create a Pod that will send the event:
    ```shell script
    kubectl apply -f - << END
    apiVersion: v1
    kind: Pod
    metadata:
      generateName: sendevents
    spec:
      containers:
       - name: sendevents
         image: gcr.io/harwayne2/sendevents@sha256:815f7f68f4b75307aec412ee4328c61376c2ec1060e0ab29a6a167b55dc533cc
         args:
           - -event-id
           - aecc3059-d97d-11e9-be0f-f4939fea3915
           - -event-type
           - dev.knative.test.event
           - -event-source
           - sender
           - -event-extensions
           - "null"
           - -event-data
           - '{"msg":"Manual test"}'
           - -event-encoding
           - binary
           - -sink
           - http://default-broker.default.svc.cluster.local
           - -add-tracing
    END
    ```
   
   
### Seeing the Trace

1. Open [Google Cloud Console](https://console.cloud.google.com/traces/traces), the trace should appear in about a minute.
