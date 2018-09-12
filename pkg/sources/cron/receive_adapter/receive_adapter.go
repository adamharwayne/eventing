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

package main

import (
	"flag"
	"fmt"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	envTarget = "TARGET"
	envEvent  = "EVENT"
	json      = "application/json"
)

type eventGenerator struct {
	logger *zap.Logger
	target string
	event  string
}

func main() {
	flag.Parse()

	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Unable to create logger: %+v", err))
	}

	target := os.Getenv(envTarget)
	event := os.Getenv(envEvent)

	eg := &eventGenerator{
		logger: logger,
		target: target,
		event:  event,
	}

	// This binary seems to send its request before the Istio sidecar is ready, let's give Istio
	// a few seconds to get setup.
	time.Sleep(15 * time.Second)

	err = eg.publish()
	if err != nil {
		logger.Error("Unable to publish event", zap.Error(err))
	}
}

func (e *eventGenerator) publish() error {
	URL := fmt.Sprintf("http://%s/", e.target)
	resp, err := http.Post(URL, json, strings.NewReader(e.event))
	if err != nil {
		e.logger.Error("Unable to http Post", zap.Error(err))
		return err
	}

	if is2xx(resp) {
		e.logger.Info("Sent event successfully", zap.Int("responseCode", resp.StatusCode))
		return nil
	}
	e.logger.Error("Bad response code", zap.Any("response", resp))
	return fmt.Errorf("bad resonpse code: %+v", resp)
}

func is2xx(resp *http.Response) bool {
	return resp.StatusCode >= http.StatusOK /* 200 */ &&
		resp.StatusCode < http.StatusMultipleChoices /* 300 */
}
