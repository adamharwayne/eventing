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
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/knative/eventing/pkg/sources"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"

	"k8s.io/api/batch/v1beta1"
	"strings"
)

const (
	// postfixReceiveAdapter is appended to the name of the service running the Receive Adapter
	postfixReceiveAdapter = "rcvadptr"
	rssURL                = "rssURL"
	schedule              = "schedule"
)

type RSSEventSource struct {
	// kubeclientset is a standard kubernetes clientset.
	kubeclientset kubernetes.Interface
	// image is the container image used as the receive adapter.
	image string
	// namespace where the feed is created.
	feedNamespace string
}

func NewRSSEventSource(kubeclientset kubernetes.Interface, feedNamespace, image string) sources.EventSource {
	glog.Infof("Creating RSS event source.")
	return &RSSEventSource{kubeclientset: kubeclientset, feedNamespace: feedNamespace, image: image}
}

func (r *RSSEventSource) StopFeed(trigger sources.EventTrigger, feedContext sources.FeedContext) error {
	glog.Infof("Stopping RSS feed with context %+v", feedContext)

	cronJobName := makeCronJobName(trigger.Resource)
	err := r.deleteReceiveAdapter(cronJobName)
	if err != nil {
		glog.Infof("Unable to delete RSS Cron Job: %v", err)
	}

	return err
}

func (r *RSSEventSource) StartFeed(trigger sources.EventTrigger, target string) (*sources.FeedContext, error) {
	glog.Infof("Creating RSS feed.")

	cronJob, err := r.createReceiveAdapter(trigger, target)
	if err != nil {
		glog.Warningf("Failed to create RSS receive adapter: %v", err)
		return nil, err
	}

	glog.Infof("Created RSS receive adapter: %+v", cronJob)

	return &sources.FeedContext{
		Context: map[string]interface{}{}}, nil
}

func makeCronJobName(resource string) string {
	serviceName := fmt.Sprintf("%s-%s-%s", "rss", resource, postfixReceiveAdapter) // TODO: this needs more UUID on the end of it.
	serviceName = strings.ToLower(serviceName)
	return serviceName
}

func (r *RSSEventSource) createReceiveAdapter(trigger sources.EventTrigger, target string) (*v1beta1.CronJob, error) {
	cc := r.kubeclientset.BatchV1beta1().CronJobs(r.feedNamespace)

	cronJobName := makeCronJobName(trigger.Resource)

	// First, check if the cron job already exists.
	if cj, err := cc.Get(cronJobName, metav1.GetOptions{}); err != nil {
		if !apierrs.IsNotFound(err) {
			glog.Infof("Cron Job get for %q failed: %s", cronJobName, err)
			return nil, err
		}
		glog.Infof("Cron Job %q doesn't exist, creating", cronJobName)
	} else {
		glog.Infof("Found existing Cron Job %q", cronJobName)
		return cj, nil
	}

	cronJob := MakeCronJob(r.feedNamespace, cronJobName, r.image, target, trigger.Parameters[schedule].(string), trigger.Parameters[rssURL].(string))
	cj, createErr := cc.Create(cronJob)
	if createErr != nil {
		glog.Errorf("Cron Job creation failed: %s", createErr)
	}
	return cj, createErr
}

func (r *RSSEventSource) deleteReceiveAdapter(cronJobName string) error {
	cc := r.kubeclientset.BatchV1beta1().CronJobs(r.feedNamespace)

	// First, check if the Cron Job actually exists.
	if _, err := cc.Get(cronJobName, metav1.GetOptions{}); err != nil {
		if apierrs.IsNotFound(err) {
			glog.Infof("Cron Job %q already deleted", cronJobName)
			return nil
		}
		glog.Infof("Cron Job Get for %q failed: %v", cronJobName, err)
		return err
	}

	return cc.Delete(cronJobName, &metav1.DeleteOptions{})
}

type parameters struct {
	Image string `json:"image,omitempty"`
}

func main() {
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	decodedParameters, _ := base64.StdEncoding.DecodeString(os.Getenv(sources.EventSourceParametersKey))

	feedNamespace := os.Getenv(sources.FeedNamespaceKey)

	var p parameters
	err := json.Unmarshal(decodedParameters, &p)
	if err != nil {
		panic(fmt.Sprintf("can not unmarshal %q : %s", decodedParameters, err))
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	sources.RunEventSource(NewRSSEventSource(kubeClient, feedNamespace, p.Image))
	log.Printf("Done...")
}
