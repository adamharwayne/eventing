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

package tracing

import (
	"fmt"

	tracingconfig "github.com/knative/eventing/pkg/tracing/config"
	"github.com/openzipkin/zipkin-go"
)

var (
	hardcodedConfig = &tracingconfig.Config{
		Enable:         true,
		Debug:          true,
		SampleRate:     1,
		ZipkinEndpoint: "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans",
	}
)

// SetupZipkinPublishing sets up Zipkin trace publishing for the process. Note that other pieces
// still need to generate the traces, this just ensures that if generated, they are collected
// appropriately. This is normally done by using tracing.HTTPSpanMiddleware as a middleware HTTP
// handler.
func SetupZipkinPublishing(serviceName string) error {
	zipkinEndpoint, err := zipkin.NewEndpoint(serviceName, "")
	if err != nil {
		return fmt.Errorf("unable to create tracing endpoint: %v", err)
	}
	oct := NewOpenCensusTracer(WithZipkinExporter(CreateZipkinReporter, zipkinEndpoint))
	// TODO: Read this from a ConfigMap, rather than hard coding it. Watch the ConfigMap for dynamic
	// updating.
	err = oct.ApplyConfig(hardcodedConfig)
	if err != nil {
		return fmt.Errorf("unable to set OpenCensusTracing config: %v", err)
	}
	return nil
}
