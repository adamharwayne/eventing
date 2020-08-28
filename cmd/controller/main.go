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

package main

import (
	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/injection"
	"knative.dev/pkg/injection/sharedmain"
	"knative.dev/pkg/signals"

	"knative.dev/eventing/pkg/reconciler/apiserversource"
	"knative.dev/eventing/pkg/reconciler/channel"
	"knative.dev/eventing/pkg/reconciler/containersource"
	"knative.dev/eventing/pkg/reconciler/eventtype"
	"knative.dev/eventing/pkg/reconciler/parallel"
	"knative.dev/eventing/pkg/reconciler/pingsource"
	"knative.dev/eventing/pkg/reconciler/sequence"
	sourcecrd "knative.dev/eventing/pkg/reconciler/source/crd"
	"knative.dev/eventing/pkg/reconciler/subscription"
)

func main() {
	ctx := signals.NewContext()
	ctx = injection.WithStartupHook(ctx, requiredCRDsOrDie)
	sharedmain.MainWithContext(ctx, "controller",
		// Messaging
		channel.NewController,
		subscription.NewController,

		// Eventing
		eventtype.NewController,

		// Flows
		parallel.NewController,
		sequence.NewController,

		// Sources
		apiserversource.NewController,
		pingsource.NewController,
		containersource.NewController,
		// Sources CRD
		sourcecrd.NewController,
	)
}

func requiredCRDsOrDie(ctx context.Context) error {
	cfg := injection.GetConfig(ctx)
	client := apiextensionsv1.NewForConfigOrDie(cfg)

	gvrs := injection.Default.GetInformerGVRs()
	for _, gvr := range gvrs {
		if isBuiltIntoK8s(gvr) {
			continue
		}
		gvrString := fmt.Sprintf("%s.%s", gvr.Resource, gvr.Group)
		_, err := client.CustomResourceDefinitions().Get(gvrString, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("unable to get %q: %w", gvrString, err)
		}
	}
	return nil
}

func isBuiltIntoK8s(gvr metav1.GroupVersionResource) bool {
	builtIntoK8s := map[string]struct{}{
		"":                          {},
		"apps":                      {},
		"autoscaling":               {},
		"batch":                     {},
		"core":                      {},
		"extensions":                {},
		"policy":                    {},
		"rbac.authorization.k8s.io": {},
		"apiextensions.k8s.io":      {},
	}
	_, present := builtIntoK8s[gvr.Group]
	return present
}
