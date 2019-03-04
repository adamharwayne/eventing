/*
 * Copyright 2019 The Knative Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package broker

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	eventingv1alpha1 "github.com/knative/eventing/pkg/apis/eventing/v1alpha1"
	"github.com/knative/eventing/pkg/provisioners"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Receiver parses Cloud Events, determines if they pass a filter, and sends them to a subscriber.
type Receiver struct {
	logger *zap.Logger
	client client.Client

	port int
	rcv  *provisioners.MessageReceiver

	dispatcher provisioners.Dispatcher
}

// New creates a new Receiver and its associated MessageReceiver. The caller is responsible for
// Start()ing the returned MessageReceiver.
func New(logger *zap.Logger, client client.Client) *Receiver {
	r := &Receiver{
		logger:     logger,
		client:     client,
		port:       8080,
		rcv:        provisioners.NewMessageReceiver(nil, logger.Sugar()),
		dispatcher: provisioners.NewMessageDispatcher(logger.Sugar()),
	}
	return r
}

var _ manager.Runnable = &Receiver{}

func (r *Receiver) Start(stopCh <-chan struct{}) error {
	err := r.initClient()
	if err != nil {
		return err
	}
	svr := r.start()
	defer r.stop(svr)

	<-stopCh
	return nil
}

func (r *Receiver) start() *http.Server {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", r.port),
		Handler: r,
	}
	r.logger.Info("Starting web server", zap.String("addr", srv.Addr))
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			r.logger.Error("HttpServer: ListenAndServe() error", zap.Error(err))
		}
	}()
	return srv
}

func (r *Receiver) stop(srv *http.Server) {
	r.logger.Info("Shutdown web server")
	if err := srv.Shutdown(nil); err != nil {
		r.logger.Error("Error shutting down the HTTP Server", zap.Error(err))
	}
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if req.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	triggerRef, err := provisioners.ParseChannel(req.Host)
	if err != nil {
		r.logger.Error("Unable to parse host as a trigger", zap.Error(err), zap.String("host", req.Host))
		w.WriteHeader(http.StatusBadRequest)
		_, err = w.Write([]byte(`{"error":"Bad Host"}`))
		if err != nil {
			r.logger.Error("Error writing the message", zap.Error(err))
		}
		return
	}

	msg, err := r.rcv.FromRequest(req)
	if err != nil {
		r.logger.Error("Error decoding HTTP Request", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	responseEvent, err := r.sendEvent(triggerRef, msg)
	if err != nil {
		r.logger.Error("Error sending the event", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if responseEvent == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	r.logger.Debug("Received a responseEvent", zap.Any("responseEvent", responseEvent))

	for n, vs := range r.dispatcher.ToHTTPHeaders(responseEvent.Headers) {
		w.Header().Del(n)
		for _, v := range vs {
			w.Header().Add(n, v)
		}
	}
	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write(msg.Payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		r.logger.Error("Error writing the response event", zap.Error(err))
		return
	}
}

// sendEvent sends an event to a subscriber if the trigger filter passes.
func (r *Receiver) sendEvent(trigger provisioners.ChannelReference, message *provisioners.Message) (*provisioners.Message, error) {
	logger := r.logger.With(zap.Any("triggerRef", trigger))
	logger.Debug("Received message")
	ctx := context.Background()

	t, err := r.getTrigger(ctx, trigger)
	if err != nil {
		logger.Info("Unable to get the Trigger", zap.Error(err))
		return nil, err
	}

	subscriberURI := t.Status.SubscriberURI
	if subscriberURI == "" {
		r.logger.Error("Unable to read subscriberURI")
		return nil, errors.New("unable to read subscriberURI")
	}

	if !shouldSendMessage(logger, &t.Spec, message) {
		logger.Debug("Message did not pass filter")
		return nil, nil
	}

	response, err := r.dispatcher.DispatchMessageWithResponse(message, subscriberURI, "", provisioners.DispatchDefaults{})
	if err != nil {
		logger.Info("Failed to dispatch message", zap.Error(err))
		return nil, err
	}
	logger.Debug("Successfully sent message")
	return response, nil
}

// Initialize the client. Mainly intended to load stuff in its cache.
func (r *Receiver) initClient() error {
	// We list triggers so that we can load the client's cache. Otherwise, on receiving an event, it
	// may not find the trigger and would return an error.
	opts := &client.ListOptions{
		// Set Raw because if we need to get more than one page, then we will put the continue token
		// into opts.Raw.Continue.
		Raw: &metav1.ListOptions{},
	}
	for {
		tl := &eventingv1alpha1.TriggerList{}
		if err := r.client.List(context.TODO(), opts, tl); err != nil {
			return err
		}
		if tl.Continue != "" {
			opts.Raw.Continue = tl.Continue
		} else {
			break
		}
	}
	return nil
}

func (r *Receiver) getTrigger(ctx context.Context, ref provisioners.ChannelReference) (*eventingv1alpha1.Trigger, error) {
	t := &eventingv1alpha1.Trigger{}
	err := r.client.Get(ctx,
		types.NamespacedName{
			Namespace: ref.Namespace,
			Name:      ref.Name,
		},
		t)
	return t, err
}

// shouldSendMessage determines whether message 'm' should be sent based on the triggerSpec 'ts'.
// Currently it supports exact matching on type and/or source of events.
func shouldSendMessage(logger *zap.Logger, ts *eventingv1alpha1.TriggerSpec, m *provisioners.Message) bool {
	if ts.Filter == nil || ts.Filter.SourceAndType == nil {
		logger.Error("No filter specified")
		return false
	}
	filterType := ts.Filter.SourceAndType.Type
	if filterType != eventingv1alpha1.TriggerAnyFilter && filterType != m.Headers["Ce-Eventtype"] {
		logger.Debug("Wrong type", zap.String("trigger.spec.filter.sourceAndType.type", filterType), zap.String("message.type", m.Headers["Ce-Eventtype"]), zap.Any("headers", m.Headers))
		return false
	}
	filterSource := ts.Filter.SourceAndType.Source
	if filterSource != eventingv1alpha1.TriggerAnyFilter && filterSource != m.Headers["Ce-Source"] {
		logger.Debug("Wrong source", zap.String("trigger.spec.filter.sourceAndType.source", filterSource), zap.String("message.source", m.Headers["Ce-Source"]))
		return false
	}
	return true
}
