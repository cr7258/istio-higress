// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package extension

import (
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	composite_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/composite/v3"
	wasm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/wasm/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	extensions "istio.io/api/extensions/v1alpha1"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/util/protoconv"
	"istio.io/istio/pkg/config/xds"
	"istio.io/istio/pkg/util/sets"
	_ "istio.io/istio/pkg/wasm" // include for registering wasm logging scope
)

// Added by Ingress
const compositeFilterType = "envoy.extensions.filters.http.composite.v3.Composite"

// End added by Ingress

var defaultConfigSource = &core.ConfigSource{
	ConfigSourceSpecifier: &core.ConfigSource_Ads{
		Ads: &core.AggregatedConfigSource{},
	},
	ResourceApiVersion: core.ApiVersion_V3,
	// we block proxy init until WasmPlugins are loaded because they might be
	// critical for security (e.g. authn/authz)
	InitialFetchTimeout: &durationpb.Duration{Seconds: 0},
}

// PopAppend takes a list of filters and a set of WASM plugins, keyed by phase. It will remove all
// plugins of a provided phase from the WASM plugin set and append them to the list of filters
func PopAppend(list []*hcm.HttpFilter,
	filterMap map[extensions.PluginPhase][]*model.WasmPluginWrapper,
	phase extensions.PluginPhase,
) []*hcm.HttpFilter {
	for _, ext := range filterMap[phase] {
		list = append(list, toEnvoyHTTPFilter(ext))
	}
	delete(filterMap, phase)
	return list
}

func toEnvoyHTTPFilter(wasmPlugin *model.WasmPluginWrapper) *hcm.HttpFilter {
	// Added by Ingress
	if wasmPlugin.FailStrategy == extensions.FailStrategy_FAIL_OPEN {
		defaultConfig, _ := anypb.New(&composite_v3.Composite{})
		return &hcm.HttpFilter{
			Name: wasmPlugin.ResourceName,
			ConfigType: &hcm.HttpFilter_ConfigDiscovery{
				ConfigDiscovery: &core.ExtensionConfigSource{
					ConfigSource:                     defaultConfigSource,
					ApplyDefaultConfigWithoutWarming: false,
					DefaultConfig:                    defaultConfig,
					TypeUrls: []string{
						xds.WasmHTTPFilterType,
						xds.RBACHTTPFilterType,
						"type.googleapis.com/" + compositeFilterType,
					},
				},
			},
		}
	}
	// End Added by Ingress
	return &hcm.HttpFilter{
		Name: wasmPlugin.ResourceName,
		ConfigType: &hcm.HttpFilter_ConfigDiscovery{
			ConfigDiscovery: &core.ExtensionConfigSource{
				ConfigSource: defaultConfigSource,
				TypeUrls: []string{
					xds.WasmHTTPFilterType,
					xds.RBACHTTPFilterType,
				},
			},
		},
	}
}

// InsertedExtensionConfigurations returns pre-generated extension configurations added via WasmPlugin.
func InsertedExtensionConfigurations(
	wasmPlugins map[extensions.PluginPhase][]*model.WasmPluginWrapper,
	names []string, pullSecrets map[string][]byte,
) []*core.TypedExtensionConfig {
	result := make([]*core.TypedExtensionConfig, 0)
	if len(wasmPlugins) == 0 {
		return result
	}
	hasName := sets.New(names...)
	for _, list := range wasmPlugins {
		for _, p := range list {
			if !hasName.Contains(p.ResourceName) {
				continue
			}
			wasmExtensionConfig := proto.Clone(p.WasmExtensionConfig).(*wasm.Wasm)
			// Find the pull secret resource name from wasm vm env variables.
			// The Wasm extension config should already have a `ISTIO_META_WASM_IMAGE_PULL_SECRET` env variable
			// at in the VM env variables, with value being the secret resource name. We try to find the actual
			// secret, and replace the env variable value with it. When ECDS config update reaches the proxy,
			// agent will extract out the secret from env variable, use it for image pulling, and strip the
			// env variable from VM config before forwarding it to envoy.
			envs := wasmExtensionConfig.GetConfig().GetVmConfig().GetEnvironmentVariables().GetKeyValues()
			secretName := envs[model.WasmSecretEnv]
			if secretName != "" {
				if sec, found := pullSecrets[secretName]; found {
					envs[model.WasmSecretEnv] = string(sec)
				} else {
					envs[model.WasmSecretEnv] = ""
				}
			}
			typedConfig := protoconv.MessageToAny(wasmExtensionConfig)
			ec := &core.TypedExtensionConfig{
				Name:        p.ResourceName,
				TypedConfig: typedConfig,
			}
			result = append(result, ec)
		}
	}
	return result
}
