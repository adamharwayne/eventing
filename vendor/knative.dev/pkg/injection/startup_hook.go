/*
Copyright 2020 The Knative Authors

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

package injection

import "golang.org/x/net/context"

type startupHookKey struct{}

// WithStartupHook adds the given hook to run during startup. If it returns an error, then startup
// fails.
func WithStartupHook(ctx context.Context, hook func(context.Context) error) context.Context {
	hooks := GetStartupHooks(ctx)
	hooks = append(hooks, hook)
	return context.WithValue(ctx, startupHookKey{}, hooks)
}

// GetStartupHooks gets all the startup hooks that have been registered.
func GetStartupHooks(ctx context.Context) []func(context.Context) error {
	value := ctx.Value(startupHookKey{})
	if value == nil {
		return []func(context.Context) error{}
	}
	return value.([]func(context.Context) error)
}
