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

package filesystem

import (
	"github.com/fsnotify/fsnotify"
	"github.com/knative/eventing/pkg/sidecar/configmap/parse"
	"github.com/knative/eventing/pkg/sidecar/multichannelfanout"
	"github.com/knative/eventing/pkg/sidecar/swappable"
	"github.com/knative/pkg/configmap"
	"go.uber.org/zap"
)

const (
	// The mount path of the configMap volume.
	ConfigDir = "/etc/config/fanout_sidecar"
)

// http.Handler that monitors an attached ConfigMap volume for updated configuration and updates its
// behavior based on the configuration.
type configMapWatcher struct {
	logger *zap.Logger
	// The directory to read the configMap from.
	dir string
	// Stop the watcher by closing this channel.
	watcherStopCh chan<- bool

	// The function to call when the configuration is updated.
	configUpdated swappable.UpdateConfig
}

// NewHandler creates a new configmaphandler.Handler.
func NewConfigMapWatcher(logger *zap.Logger, dir string, updateConfig swappable.UpdateConfig) (chan<- bool, error) {
	conf, err := readConfigMap(logger, dir)
	if err != nil {
		logger.Error("Unable to read configMap", zap.Error(err))
		return nil, err
	}

	logger.Info("Read initial configMap", zap.Any("conf", conf))

	err = updateConfig(conf)
	if err != nil {
		logger.Error("Unable to use the initial configMap: %v", zap.Error(err))
		return nil, err
	}

	cmh := &configMapWatcher{
		logger: logger,
		dir:    dir,
		configUpdated: updateConfig,
	}
	watcherStopCh, err := cmh.startWatcher(dir)
	if err != nil {
		logger.Error("Unable to start the configMap file watcher", zap.Error(err))
		return nil, err
	}
	return watcherStopCh, nil
}

// readConfigMap attempts to read the configMap from the attached volume.
func readConfigMap(logger *zap.Logger, dir string) (*multichannelfanout.Config, error) {
	cm, err := configmap.Load(dir)
	if err != nil {
		logger.Error("Unable to read configMap", zap.Error(err))
		return nil, err
	}
	return parse.ConfigMapData(logger, cm)
}

// readConfigMapAndUpdateSubs reads the configMap data and updates configuration of cmh.fanout, if
// it has changed.
//
// Note that this is often called multiple times when the configMap is updated, so it should do its
// best to discard redundant calls.
func (cmh *configMapWatcher) readConfigMapAndUpdateConfig() {
	conf, err := readConfigMap(cmh.logger, cmh.dir)
	if err != nil {
		cmh.logger.Error("Unable to read the configMap", zap.Error(err))
		return
	}
	err = cmh.configUpdated(conf)
	if err != nil {
		cmh.logger.Error("Unable to update config", zap.Error(err))
		return
	}
}

// startWatcher starts a background go routine that gets events when the filesystem in configDir is
// changed.
func (cmh *configMapWatcher) startWatcher(dir string) (chan<- bool, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	stopCh := make(chan bool)
	go func() {
		for {
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					// Channel closed.
					cmh.logger.Error("watcher.Events channel closed") // TODO: Should this be fatal?
					return
				}
				cmh.readConfigMapAndUpdateConfig()
			case err, ok := <-watcher.Errors:
				if !ok {
					// Channel closed.
					cmh.logger.Error("watcher.Errors channel closed") // TODO: Should this be fatal?
					return
				}
				cmh.logger.Error("watcher.Errors", zap.Error(err))
			case _, ok := <-stopCh:
				if !ok {
					// stopCh has been closed
					return
				}
			}
		}
	}()

	return stopCh, watcher.Add(dir)
}
