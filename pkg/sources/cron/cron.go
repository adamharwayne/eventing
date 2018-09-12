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
	"github.com/knative/eventing/pkg/sources"
	"go.uber.org/zap"
	"k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strings"
)

const (
	sidecarIstioInjectAnnotation = "sidecar.istio.io/inject"
	schedule                     = "schedule"
	event                        = "event"
)

// This is a simple source that cronmatically generates events based on a CronJob. It is intended
// to be used for testing other portions of the Eventing system.

type cronEventSource struct {
	logger *zap.Logger
	// kubeclientset is a standard kubernetes clientset.
	kubeclientset kubernetes.Interface

	feedNamespace      string
	feedServiceAccount string

	image string
}

func newCronEventSource(logger *zap.Logger, kubeclientset kubernetes.Interface, feedNamespace, feedServiceAccount, image string) sources.EventSource {
	logger.Info("Creating Cron Event Source")
	return &cronEventSource{
		logger:             logger,
		kubeclientset:      kubeclientset,
		feedNamespace:      feedNamespace,
		feedServiceAccount: feedServiceAccount,
		image:              image,
	}
}

func (c *cronEventSource) StartFeed(trigger sources.EventTrigger, target string) (*sources.FeedContext, error) {
	c.logger.Info("Starting Cron Event Source")
	cronJob, err := c.createCronJob(trigger, target)
	if err != nil {
		c.logger.Warn("Failed to create Cron Event Source Cron Job", zap.Error(err))
		return nil, err
	}
	c.logger.Info("Created Cron Event Source Cron Job", zap.Any("cronJob", cronJob))
	return &sources.FeedContext{
		Context: map[string]interface{}{},
	}, nil
}

func (c *cronEventSource) StopFeed(trigger sources.EventTrigger, feedContext sources.FeedContext) error {
	c.logger.Info("Stopping Cron Event Source", zap.Any("feedContext", feedContext))

	cronJobName := makeCronJobName(trigger.Resource)
	err := c.deleteCronJob(cronJobName)
	if err != nil {
		c.logger.Error("Unable to stop Cron Event Source Cron Jobs", zap.Error(err))
	}
	return err
}

func makeCronJobName(resource string) string {
	n := fmt.Sprintf("%s-%s", "cronevent", resource)
	n = strings.ToLower(n)
	return n
}

func (c *cronEventSource) createCronJob(trigger sources.EventTrigger, target string) (*v1beta1.CronJob, error) {
	cc := c.kubeclientset.BatchV1beta1().CronJobs(c.feedNamespace)

	cronJobName := makeCronJobName(trigger.Resource)

	// First, check if the cron job already exists.
	if cj, err := cc.Get(cronJobName, metav1.GetOptions{}); err != nil {
		if !apierrs.IsNotFound(err) {
			c.logger.Info("CronJob.Get failed", zap.Error(err), zap.String("cronJobName", cronJobName))
			return nil, err
		}
		c.logger.Info("CronJob doesn't exist, creating it", zap.String("cronJobName", cronJobName))
	} else {
		c.logger.Info("Found existing CronJob", zap.String("cronJobName", cronJobName))
		return cj, nil
	}

	cronJob := c.makeCronJob(cronJobName, target,
		trigger.Parameters[schedule].(string), trigger.Parameters[event].(string))
	cj, createErr := cc.Create(cronJob)
	if createErr != nil {
		c.logger.Info("CronJob.Create failed", zap.Error(createErr), zap.Any("cronJob", cronJob))
	}
	return cj, createErr
}

func (c *cronEventSource) deleteCronJob(cronJobName string) error {
	cc := c.kubeclientset.BatchV1beta1().CronJobs(c.feedNamespace)

	// First, check if the Cron Job actually exists.
	if _, err := cc.Get(cronJobName, metav1.GetOptions{}); err != nil {
		if apierrs.IsNotFound(err) {
			c.logger.Info("Cron Job already deleted.", zap.String("cronJobName", cronJobName))
			return nil
		}
		c.logger.Info("CronJob.Get failed", zap.Error(err), zap.String("cronJobName", cronJobName))
		return err
	}

	// It seems to default to 'orphan', which will leave running jobs for the user to delete
	// manually. Setting to foreground should force them to be deleted.
	fgDeletion := metav1.DeletePropagationForeground
	return cc.Delete(cronJobName, &metav1.DeleteOptions{
		PropagationPolicy: &fgDeletion,
	})
}

func (c *cronEventSource) makeCronJob(name, target, schedule, event string) *v1beta1.CronJob {
	labels := map[string]string{
		"receive-adapter": "cron",
	}
	return &v1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.feedNamespace,
			Labels:    labels,
		},
		Spec: v1beta1.CronJobSpec{
			Schedule:          schedule,
			ConcurrencyPolicy: v1beta1.ReplaceConcurrent,
			JobTemplate: v1beta1.JobTemplateSpec{
				Spec: v1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
							// Inject Istio so any connection made from the receive adapter
							// goes through and is enforced by Istio.
							Annotations: map[string]string{sidecarIstioInjectAnnotation: "true"},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: c.feedServiceAccount,
							Containers: []corev1.Container{
								{
									Name:  "cron-event-generator",
									Image: c.image,
									Env: []corev1.EnvVar{
										{
											Name:  "TARGET",
											Value: target,
										},
										{
											Name:  "EVENT",
											Value: event,
										},
									},
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
		},
	}
}

type parameters struct {
	Image    string
	Schedule string
	Event    string
}

func main() {
	flag.Parse()
	flag.Lookup("logtostderr").Value.Set("true")

	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("unable to create logger: %+v", err))
	}

	decodedParameters, _ := base64.StdEncoding.DecodeString(os.Getenv(sources.EventSourceParametersKey))

	feedNamespace := os.Getenv(sources.FeedNamespaceKey)
	feedServiceAccount := os.Getenv(sources.FeedServiceAccountKey)

	var p parameters
	err = json.Unmarshal(decodedParameters, &p)
	if err != nil {
		panic(fmt.Sprintf("can not unmarshal %q : %s", decodedParameters, err))
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		logger.Fatal("Error building kubeconfig", zap.Error(err))
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatal("Error building kubernetes clientset", zap.Error(err))
	}

	sources.RunEventSource(newCronEventSource(logger, kubeClient, feedNamespace, feedServiceAccount, p.Image))
	logger.Info("Done...")
}
