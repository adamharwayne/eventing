/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helpers

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/openzipkin/zipkin-go/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"knative.dev/eventing/test/base/resources"
	"knative.dev/eventing/test/common"
	tracinghelper "knative.dev/eventing/test/conformance/helpers/tracing"
	"knative.dev/pkg/test/zipkin"
)

// TracingTestCase is the test case information for tracing tests.
type TracingTestCase struct {
	// IncomingTraceId controls whether the original request is sent to the Broker/Channel already
	// has a trace ID associated with it by the sender.
	IncomingTraceId bool
	// Istio controls whether the Pods being created for the test (sender, transformer, logger,
	// etc.) have Istio sidecars. It does not affect the Channel Pods.
	Istio bool
}

func ChannelTracingWithChannelTestRunner(t *testing.T, channelTestRunner common.ChannelTestRunner) {
	testCases := map[string]TracingTestCase{
		"includes incoming trace id": {
			IncomingTraceId: true,
		},
	}

	for n, tc := range testCases {
		t.Run(n, func(t *testing.T) {
			channelTestRunner.RunTests(t, common.FeatureBasic, func(st *testing.T, channel string) {
				// Don't accidentally use t, use st instead. To ensure this, shadow 't' to a useless
				// type.
				t := struct{}{}
				_ = fmt.Sprintf("%s", t)

				channelTypeMeta := common.GetChannelTypeMeta(channel)
				ChannelTracingTest(st, *channelTypeMeta, tc)
			})
		})
	}
}

// ChannelTracingTest tests a specific channel type with the given test case. Verifying that tracing
// is correct between the channel's input and its subscriber.
func ChannelTracingTest(t *testing.T, channel metav1.TypeMeta, tc TracingTestCase) {
	client := common.Setup(t, true)
	defer common.TearDown(client)

	loggerPodName := "logger"

	// Do NOT call zipkin.CleanupZipkinTracingSetup. That will be called exactly once in TestMain.
	tracinghelper.Setup(t, client)

	expected, mustContain := setupChannelTracing(t, &channel, client, loggerPodName, tc.IncomingTraceId)
	assertLogContents(t, client, loggerPodName, mustContain)

	traceID := getTraceID(t, client, loggerPodName)
	trace, err := zipkin.JSONTrace(traceID, expected.SpanCount(), 60*time.Second)
	if err != nil {
		t.Fatalf("Unable to get trace %q: %v. Trace so far %+v", traceID, err, tracinghelper.PrettyPrintTrace(trace))
	}
	t.Logf("I got the trace, %q!\n%+v", traceID, tracinghelper.PrettyPrintTrace(trace))

	tree := tracinghelper.GetTraceTree(t, trace)
	if err := expected.Matches(tree); err != nil {
		t.Fatalf("Trace Tree did not match expected: %v", err)
	}
}

// setupChannelTracing is the general setup for TestChannelTracing. It creates the following:
// SendEvents (Pod) -> Channel -> Subscription -> K8s Service -> LogEvents (Pod)
// It returns the expected trace tree and a string that is expected to be sent by the SendEvents Pod
// and should be present in the LogEvents Pod logs.
func setupChannelTracing(t *testing.T, channel *metav1.TypeMeta, client *common.Client, loggerPodName string, incomingTraceId bool) (tracinghelper.TestSpanTree, string) {
	// Create the Channel.
	channelName := "ch"
	client.CreateChannelOrFail(channelName, channel)

	// Create the 'sink', a LogEvents Pod and a K8s Service that points to it.
	loggerPod := resources.EventDetailsPod(loggerPodName)
	client.CreatePodOrFail(loggerPod, common.WithService(loggerPodName))

	// Create the Subscription linking the Channel to the LogEvents K8s Service.
	client.CreateSubscriptionOrFail(
		"sub",
		channelName,
		channel,
		resources.WithSubscriberForSubscription(loggerPodName),
	)

	// Wait for all test resources to be ready, so that we can start sending events.
	if err := client.WaitForAllTestResourcesReady(); err != nil {
		t.Fatalf("Failed to get all test resources ready: %v", err)
	}

	// Everything is setup to receive an event. Generate a CloudEvent.
	senderName := "sender"
	eventID := fmt.Sprintf("%s", uuid.NewUUID())
	body := fmt.Sprintf("TestChannelTracing %s", eventID)
	event := &resources.CloudEvent{
		ID:       eventID,
		Source:   senderName,
		Type:     resources.CloudEventDefaultType,
		Data:     fmt.Sprintf(`{"msg":%q}`, body),
		Encoding: resources.CloudEventEncodingBinary,
	}

	// Send the CloudEvent (either with or without tracing inside the SendEvents Pod).
	sendEvent := client.SendFakeEventToAddressable
	if incomingTraceId {
		sendEvent = client.SendFakeEventWithTracingToAddressable
	}
	if err := sendEvent(senderName, channelName, channel, event); err != nil {
		t.Fatalf("Failed to send fake CloudEvent to the channel %q", channelName)
	}

	// We expect the following spans:
	// 0. Artificial root span.
	// 1. Sending pod sends event to Channel (only if the sending pod generates a span).
	// 2. Channel receives event from sending pod.
	// 3. Channel sends event to logging pod.
	// 4. Logging pod receives event from Channel.
	expected := tracinghelper.TestSpanTree{
		// 0. Artificial root span.
		Root: true,
		// 1 is added below if it is needed.
		Children: []tracinghelper.TestSpanTree{
			{
				// 2. Channel receives event from sending pod.
				Kind: model.Server,
				Tags: map[string]string{
					"http.method":      "POST",
					"http.status_code": "202",
					"http.host":        fmt.Sprintf("%s-kn-channel.%s.svc.cluster.local", channelName, client.Namespace),
					"http.path":        "/",
				},
				Children: []tracinghelper.TestSpanTree{
					{
						// 3. Channel sends event to logging pod.
						Kind: model.Client,
						Tags: map[string]string{
							"http.method":      "POST",
							"http.path":        "/",
							"http.status_code": "202",
							"http.url":         fmt.Sprintf("http://%s.%s.svc.cluster.local/", loggerPod.Name, client.Namespace),
						},
						Children: []tracinghelper.TestSpanTree{
							{
								// 4. Logging pod receives event from Channel.
								Kind:                     model.Server,
								LocalEndpointServiceName: "logger",
								Tags: map[string]string{
									"http.method":      "POST",
									"http.path":        "/",
									"http.status_code": "202",
									"http.host":        fmt.Sprintf("%s.%s.svc.cluster.local", loggerPod.Name, client.Namespace),
								},
							},
						},
					},
				},
			},
		},
	}

	if incomingTraceId {
		expected.Children = []tracinghelper.TestSpanTree{
			{
				// 1. Sending pod sends event to Channel (only if the sending pod generates a span).
				Kind:                     model.Client,
				LocalEndpointServiceName: "sender",
				Tags: map[string]string{
					"http.method":      "POST",
					"http.status_code": "202",
					"http.url":         fmt.Sprintf("http://%s-kn-channel.%s.svc.cluster.local", channelName, client.Namespace),
				},
				Children: expected.Children,
			},
		}
	}
	return expected, body
}

// assertLogContents verifies that loggerPodName's logs contains mustContain. It is used to show
// that the expected event was sent to the logger Pod.
func assertLogContents(t *testing.T, client *common.Client, loggerPodName string, mustContain string) {
	if err := client.CheckLog(loggerPodName, common.CheckerContains(mustContain)); err != nil {
		t.Fatalf("String %q not found in logs of logger pod %q: %v", mustContain, loggerPodName, err)
	}
}

// getTraceID gets the TraceID from loggerPodName's logs. It will also assert that body is present.
func getTraceID(t *testing.T, client *common.Client, loggerPodName string) string {
	logs, err := client.GetLog(loggerPodName)
	if err != nil {
		t.Fatalf("Error getting logs: %v", err)
	}
	// This is the format that the eventdetails image prints headers.
	re := regexp.MustCompile("\nGot Header X-B3-Traceid: ([a-zA-Z0-9]{32})\n")
	matches := re.FindStringSubmatch(logs)
	if len(matches) != 2 {
		t.Fatalf("Unable to extract traceID: %q", logs)
	}
	traceID := matches[1]
	return traceID
}

func ChannelTracingWithReplyWithChannelTestRunner(t *testing.T, channelTestRunner common.ChannelTestRunner) {
	testCases := map[string]TracingTestCase{
		"includes incoming trace id": {
			IncomingTraceId: true,
		},
	}

	for n, tc := range testCases {
		t.Run(n, func(t *testing.T) {
			channelTestRunner.RunTests(t, common.FeatureBasic, func(st *testing.T, channel string) {
				// Don't accidentally use t, use st instead. To ensure this, shadow 't' to a useless
				// type.
				t := struct{}{}
				_ = fmt.Sprintf("%s", t)

				channelTypeMeta := common.GetChannelTypeMeta(channel)
				ChannelTracingWithReplyTest(st, *channelTypeMeta, tc)
			})
		})
	}
}

// ChannelTracingWithReplyTest tests a specific channel type with the given test case. Verifying
// that tracing is correct between the channel's input, its subscriber, and its reply.
func ChannelTracingWithReplyTest(t *testing.T, channel metav1.TypeMeta, tc TracingTestCase) {
	client := common.Setup(t, true)
	defer common.TearDown(client)

	loggerPodName := "logger"

	// Do NOT call zipkin.CleanupZipkinTracingSetup. That will be called exactly once in
	// TestMain.
	tracinghelper.Setup(t, client)

	expected, mustContain := setupChannelTracingWithReply(t, &channel, client, loggerPodName, tc.IncomingTraceId)
	assertLogContents(t, client, loggerPodName, mustContain)

	traceID := getTraceID(t, client, loggerPodName)
	trace, err := zipkin.JSONTrace(traceID, expected.SpanCount(), 60*time.Second)
	if err != nil {
		t.Fatalf("Unable to get trace %q: %v. Trace so far %+v", traceID, err, tracinghelper.PrettyPrintTrace(trace))
	}
	t.Logf("I got the trace, %q!\n%+v", traceID, tracinghelper.PrettyPrintTrace(trace))

	tree := tracinghelper.GetTraceTree(t, trace)
	if err := expected.Matches(tree); err != nil {
		t.Fatalf("Trace Tree did not match expected: %v", err)
	}
}

// setupChannelTracing is the general setup for TestChannelTracing. It creates the following:
// SendEvents (Pod) -> Channel -> Subscription -> K8s Service -> Mutate (Pod)
//                                                                   v
// LogEvents (Pod) <- K8s Service <- Subscription  <- Channel <- (Reply) Subscription
// It returns the expected trace tree and a string that is expected to be sent by the SendEvents Pod
// and should be present in the LogEvents Pod logs.
func setupChannelTracingWithReply(t *testing.T, channel *metav1.TypeMeta, client *common.Client, loggerPodName string, incomingTraceId bool) (tracinghelper.TestSpanTree, string) {
	// Create the Channels.
	channelName := "ch"
	client.CreateChannelOrFail(channelName, channel)

	replyChannelName := "reply-ch"
	client.CreateChannelOrFail(replyChannelName, channel)

	// Create the 'sink', a LogEvents Pod and a K8s Service that points to it.
	loggerPod := resources.EventDetailsPod(loggerPodName)
	client.CreatePodOrFail(loggerPod, common.WithService(loggerPodName))

	// Create the subscriber, a Pod that mutates the event.
	transformerPod := resources.EventTransformationPod("transformer", &resources.CloudEvent{
		Type: "mutated",
	})
	client.CreatePodOrFail(transformerPod, common.WithService(transformerPod.Name))

	// Create the Subscription linking the Channel to the mutator.
	client.CreateSubscriptionOrFail(
		"sub",
		channelName,
		channel,
		resources.WithSubscriberForSubscription(transformerPod.Name),
		resources.WithReplyForSubscription(replyChannelName, channel))

	// Create the Subscription linking the reply Channel to the LogEvents K8s Service.
	client.CreateSubscriptionOrFail(
		"reply-sub",
		replyChannelName,
		channel,
		resources.WithSubscriberForSubscription(loggerPodName),
	)

	// Wait for all test resources to be ready, so that we can start sending events.
	if err := client.WaitForAllTestResourcesReady(); err != nil {
		t.Fatalf("Failed to get all test resources ready: %v", err)
	}

	// Everything is setup to receive an event. Generate a CloudEvent.
	senderName := "sender"
	eventID := fmt.Sprintf("%s", uuid.NewUUID())
	body := fmt.Sprintf("TestChannelTracing %s", eventID)
	event := &resources.CloudEvent{
		ID:       eventID,
		Source:   senderName,
		Type:     resources.CloudEventDefaultType,
		Data:     fmt.Sprintf(`{"msg":%q}`, body),
		Encoding: resources.CloudEventEncodingBinary,
	}

	// Send the CloudEvent (either with or without tracing inside the SendEvents Pod).
	sendEvent := client.SendFakeEventToAddressable
	if incomingTraceId {
		sendEvent = client.SendFakeEventWithTracingToAddressable
	}
	if err := sendEvent(senderName, channelName, channel, event); err != nil {
		t.Fatalf("Failed to send fake CloudEvent to the channel %q", channelName)
	}

	// We expect the following spans:
	// 0. Artificial root span.
	// 1. Sending pod sends event to Channel (only if the sending pod generates a span).
	// 2. Channel receives event from sending pod.
	// 3. Channel sends event to transformer pod.
	// 4. Transformer Pod receives event from Channel.
	// 5. Channel sends reply from Transformer Pod to the reply Channel.
	// 6. Reply Channel receives event from the original Channel's reply.
	// 7. Reply Channel sends event to the logging Pod.
	// 8. Logging pod receives event from Channel.
	expected := tracinghelper.TestSpanTree{
		// 0. Artificial root span.
		Root: true,
		// 1 is added below if it is needed.
		Children: []tracinghelper.TestSpanTree{
			{
				// 2. Channel receives event from sending pod.
				Kind: model.Server,
				Tags: map[string]string{
					"http.method":      "POST",
					"http.status_code": "202",
					"http.host":        fmt.Sprintf("%s-kn-channel.%s.svc.cluster.local", channelName, client.Namespace),
					"http.path":        "/",
				},
				Children: []tracinghelper.TestSpanTree{
					{
						// 3. Channel sends event to transformer pod.
						Kind: model.Client,
						Tags: map[string]string{
							"http.method":      "POST",
							"http.status_code": "200",
							"http.url":         fmt.Sprintf("http://%s.%s.svc.cluster.local/", transformerPod.Name, client.Namespace),
						},
						Children: []tracinghelper.TestSpanTree{
							{
								// 4. Transformer Pod receives event from Channel.
								Kind:                     model.Server,
								LocalEndpointServiceName: transformerPod.Name,
								Tags: map[string]string{
									"http.method":      "POST",
									"http.path":        "/",
									"http.status_code": "200",
									"http.host":        fmt.Sprintf("%s.%s.svc.cluster.local", transformerPod.Name, client.Namespace),
								},
							},
						},
					},
					{
						// 5. Channel sends reply from Transformer Pod to the reply Channel.
						Kind: model.Client,
						Tags: map[string]string{
							"http.method":      "POST",
							"http.status_code": "202",
							"http.url":         fmt.Sprintf("http://%s-kn-channel.%s.svc.cluster.local", replyChannelName, client.Namespace),
						},
						Children: []tracinghelper.TestSpanTree{
							// 6. Reply Channel receives event from the original Channel's reply.
							{
								Kind: model.Server,
								Tags: map[string]string{
									"http.method":      "POST",
									"http.status_code": "202",
									"http.host":        fmt.Sprintf("%s-kn-channel.%s.svc.cluster.local", replyChannelName, client.Namespace),
									"http.path":        "/",
								},
								Children: []tracinghelper.TestSpanTree{
									{
										// 7. Reply Channel sends event to the logging Pod.
										Kind: model.Client,
										Tags: map[string]string{
											"http.method":      "POST",
											"http.status_code": "202",
											"http.url":         fmt.Sprintf("http://%s.%s.svc.cluster.local/", loggerPod.Name, client.Namespace),
										},
										Children: []tracinghelper.TestSpanTree{
											{
												// 8. Logging pod receives event from Channel.
												Kind:                     model.Server,
												LocalEndpointServiceName: loggerPod.Name,
												Tags: map[string]string{
													"http.method":      "POST",
													"http.path":        "/",
													"http.status_code": "202",
													"http.host":        fmt.Sprintf("%s.%s.svc.cluster.local", loggerPod.Name, client.Namespace),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if incomingTraceId {
		expected.Children = []tracinghelper.TestSpanTree{
			{
				// 1. Sending pod sends event to Channel (only if the sending pod generates a span).
				Kind:                     model.Client,
				LocalEndpointServiceName: "sender",
				Tags: map[string]string{
					"http.method":      "POST",
					"http.status_code": "202",
					"http.url":         fmt.Sprintf("http://%s-kn-channel.%s.svc.cluster.local", channelName, client.Namespace),
				},
				Children: expected.Children,
			},
		}
	}
	return expected, body
}
