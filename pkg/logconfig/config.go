/*
Copyright 2018 The Knative Authors

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

package logconfig

import (
	"fmt"
	"os"

	"github.com/knative/pkg/logging"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConfigMapNameEnv is the environment value to get the name of the config map used for knative-eventing logging config.
	ConfigMapNameEnv = "CONFIG_LOGGING_NAME"

	// Named Loggers are used to override the default log level. config-logging.yaml will use the follow:
	//
	// loglevel.controller: "info"
	// loglevel.webhook: "info"

	// Controller is the name of the override key used inside of the logging config for Controller.
	Controller = "controller"

	// SourcesController is the name of the override key used inside of the logging config for Sources Controller.
	SourcesController = "sources-controller"

	// BrokerFilter is the name of the override key used inside of the logging config for the Broker
	// Filter.
	BrokerFilter = "broker-filter"

	// WebhookNameEnv is the name of the override key used inside of the logging config for Webhook
	// Controller.
	WebhookNameEnv = "WEBHOOK_NAME"
)

func ConfigMapName() string {
	if cm := os.Getenv(ConfigMapNameEnv); cm != "" {
		return cm
	}

	panic(fmt.Sprintf(`The environment variable %q is not set

If this is a process running on Kubernetes, then it should be using the downward
API to initialize this variable via:

  env:
  - name: CONFIG_LOGGING_NAME
    value: config-logging
`, ConfigMapNameEnv))
}

func WebhookName() string {
	if webhook := os.Getenv(WebhookNameEnv); webhook != "" {
		return webhook
	}

	panic(fmt.Sprintf(`The environment variable %q is not set

If this is a process running on Kubernetes, then it should be using the downward
API to initialize this variable via:

  env:
  - name: WEBHOOK_NAME
    value: webhook
`, WebhookNameEnv))
}

func DefaultLoggingConfig() (v1.ConfigMap, error) {
	loggingConfig, err := logging.NewConfigFromMap(map[string]string{})
	if err != nil {
		return v1.ConfigMap{}, err
	}
	return v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: ConfigMapName(),
		},
		Data: map[string]string{
			"zap-logging-config": loggingConfig.LoggingConfig,
		},
	}, nil
}
