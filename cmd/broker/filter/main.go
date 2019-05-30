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

package main

import (
	"flag"

	eventingv1alpha1 "github.com/knative/eventing/pkg/apis/eventing/v1alpha1"
	"github.com/knative/eventing/pkg/broker"
	"github.com/knative/eventing/pkg/logconfig"
	"github.com/knative/eventing/pkg/logging"
	"github.com/knative/eventing/pkg/tracing"
	"github.com/knative/eventing/pkg/utils"
	"github.com/knative/pkg/configmap"
	"github.com/knative/pkg/signals"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// NAMESPACE is the name of the environment variable containing the namespace this binary is
	// running in.
	NAMESPACE = "NAMESPACE"
)

func main() {
	loggingConfig := logging.NewConfig()
	sugaredLogger, logLevel := logging.NewLoggerFromConfig(loggingConfig, logconfig.BrokerFilter)
	logger := sugaredLogger.Desugar()
	defer logger.Sync()

	flag.Parse()

	logger.Info("Starting...")

	ns := utils.GetRequiredEnvOrFatal(NAMESPACE)
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		Namespace: ns,
	})
	if err != nil {
		logger.Fatal("Error starting up.", zap.Error(err))
	}

	if err = eventingv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		logger.Fatal("Unable to add eventingv1alpha1 scheme", zap.Error(err))
	}

	kc := kubernetes.NewForConfigOrDie(mgr.GetConfig())
	configMapWatcher := configmap.NewInformedWatcher(kc, ns)

	if err = tracing.SetupDynamicZipkinPublishing(logger.Sugar(), ns, configMapWatcher, utils.GetRequiredEnvOrFatal("ZIPKIN_SERVICE_NAME")); err != nil {
		logger.Fatal("Error setting up Zipkin publishing", zap.Error(err))
	}

	configMapWatcher.WatchWithDefault(logconfig.DefaultLoggingConfig(), logging.UpdateLevelFromConfigMap(logger.Sugar(), logLevel, logconfig.BrokerFilter))

	// We are running both the receiver (takes messages in from the Broker) and the dispatcher (send
	// the messages to the triggers' subscribers) in this binary.
	receiver, err := broker.New(logger, mgr.GetClient())
	if err != nil {
		logger.Fatal("Error creating Receiver", zap.Error(err))
	}
	err = mgr.Add(receiver)
	if err != nil {
		logger.Fatal("Unable to start the receiver", zap.Error(err), zap.Any("receiver", receiver))
	}

	// Set up signals so we handle the first shutdown signal gracefully.
	stopCh := signals.SetupSignalHandler()

	// configMapWatcher does not block, so start it first.
	if err = configMapWatcher.Start(stopCh); err != nil {
		logger.Fatal("Failed to start ConfigMap watcher", zap.Error(err))
	}

	// Start blocks forever.
	logger.Info("Manager starting...")
	err = mgr.Start(stopCh)
	if err != nil {
		logger.Fatal("Manager.Start() returned an error", zap.Error(err))
	}
	logger.Info("Exiting...")
}
