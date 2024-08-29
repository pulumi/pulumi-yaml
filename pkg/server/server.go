// Copyright 2022, Pulumi Corporation.
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

// pulumi-language-yaml is the "language host" for Pulumi programs written in YAML or JSON. It is responsible for
// evaluating JSON/YAML templates, registering resources, outputs, and so on, with the Pulumi engine.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	pbempty "github.com/golang/protobuf/ptypes/empty"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/version"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

// yamlLanguageHost implements the LanguageRuntimeServer interface
// for use as an API endpoint.
type yamlLanguageHost struct {
	pulumirpc.UnimplementedLanguageRuntimeServer

	engineAddress string
	tracing       string
	compiler      string
	template      *ast.TemplateDecl
	diags         syntax.Diagnostics
}

func NewLanguageHost(engineAddress, tracing string, compiler string) pulumirpc.LanguageRuntimeServer {
	return &yamlLanguageHost{
		engineAddress: engineAddress,
		tracing:       tracing,
		compiler:      compiler,
	}
}

func (host *yamlLanguageHost) loadTemplate(compilerEnv []string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	if host.template != nil && host.compiler == "" {
		return host.template, host.diags, nil
	}

	var template *ast.TemplateDecl
	var diags syntax.Diagnostics
	var err error
	if host.compiler == "" {
		template, diags, err = pulumiyaml.Load()
	} else {
		template, diags, err = pulumiyaml.LoadFromCompiler(host.compiler, "", compilerEnv)
	}
	if err != nil {
		return nil, diags, err
	}
	if diags.HasErrors() {
		return nil, diags, nil
	}
	host.template = template
	host.diags = diags

	return host.template, diags, nil
}

// GetRequiredPlugins computes the complete set of anticipated plugins required by a program.
func (host *yamlLanguageHost) GetRequiredPlugins(ctx context.Context,
	req *pulumirpc.GetRequiredPluginsRequest,
) (*pulumirpc.GetRequiredPluginsResponse, error) {
	template, diags, err := host.loadTemplate(nil)
	if err != nil {
		return nil, err
	}
	if diags.HasErrors() {
		return nil, diags
	}

	pkgs, pluginDiags := pulumiyaml.GetReferencedPlugins(template)
	diags.Extend(pluginDiags...)
	if diags.HasErrors() {
		// We currently swallow the error to allow project config to evaluate
		// Specifically, if one sets a config key via the CLI but not within the `config` block
		// of their YAML program, it would error.
		return &pulumirpc.GetRequiredPluginsResponse{}, nil
	}
	var plugins []*pulumirpc.PluginDependency
	for _, pkg := range pkgs {
		plugins = append(plugins, &pulumirpc.PluginDependency{
			Kind:    string(apitype.ResourcePlugin),
			Name:    pkg.Package,
			Version: pkg.Version,
			Server:  pkg.PluginDownloadURL,
		})
	}
	return &pulumirpc.GetRequiredPluginsResponse{
		Plugins: plugins,
	}, nil
}

// RPC endpoint for LanguageRuntimeServer::Run. This actually evaluates the JSON-based project.
func (host *yamlLanguageHost) Run(ctx context.Context, req *pulumirpc.RunRequest) (*pulumirpc.RunResponse, error) {
	if pwd := req.GetPwd(); pwd != "" {
		err := os.Chdir(pwd)
		if err != nil {
			return nil, err
		}
	}

	configValue := req.GetConfig()
	jsonConfigValue, err := json.Marshal(configValue)
	if err != nil {
		return nil, err
	}

	compilerEnv := []string{
		fmt.Sprintf(`PULUMI_STACK=%s`, req.GetStack()),
		fmt.Sprintf(`PULUMI_ORGANIZATION=%s`, req.GetOrganization()),
		fmt.Sprintf(`PULUMI_PROJECT=%s`, req.GetProject()),
		fmt.Sprintf(`PULUMI_CONFIG=%s`, jsonConfigValue),
	}

	template, diags, err := host.loadTemplate(compilerEnv)
	if err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}

	diagWriter := template.NewDiagnosticWriter(os.Stderr, 0, true)
	if len(diags) != 0 {
		err := diagWriter.WriteDiagnostics(diags.HCL())
		if err != nil {
			return nil, err
		}
	}
	if diags.HasErrors() {
		return &pulumirpc.RunResponse{Error: "failed to load template"}, nil
	}

	confPropMap, err := plugin.UnmarshalProperties(req.GetConfigPropertyMap(),
		plugin.MarshalOptions{KeepUnknowns: true, KeepSecrets: true, SkipInternalKeys: true})
	if err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}

	// Use the Pulumi Go SDK to create an execution context and to interact with the engine.
	// This encapsulates a fair bit of the boilerplate otherwise needed to do RPCs, etc.
	pctx, err := pulumi.NewContext(ctx, pulumi.RunInfo{
		Project:           req.GetProject(),
		Stack:             req.GetStack(),
		Config:            req.GetConfig(),
		ConfigSecretKeys:  req.GetConfigSecretKeys(),
		ConfigPropertyMap: confPropMap,
		Organization:      req.Organization,
		Parallel:          req.GetParallel(),
		DryRun:            req.GetDryRun(),
		MonitorAddr:       req.GetMonitorAddress(),
		EngineAddr:        host.engineAddress,
	})
	if err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}
	defer pctx.Close()
	// Now instruct the Pulumi Go SDK to run the pulumi YAML interpreter.
	if err := pulumi.RunWithContext(pctx, func(ctx *pulumi.Context) error {
		loaderClient, err := schema.NewLoaderClient(req.LoaderTarget)
		if err != nil {
			return err
		}
		defer loaderClient.Close()
		loader := pulumiyaml.NewPackageLoaderFromSchemaLoader(loaderClient)

		// Now "evaluate" the template.
		return pulumiyaml.RunTemplate(pctx, template, req.GetConfig(), confPropMap, loader)
	}); err != nil {
		if diags, ok := pulumiyaml.HasDiagnostics(err); ok {
			err := diagWriter.WriteDiagnostics(diags.Unshown().HCL())
			if err != nil {
				return nil, err
			}
			if diags.HasErrors() {
				return &pulumirpc.RunResponse{Error: "", Bail: true}, nil
			}
			return &pulumirpc.RunResponse{}, nil
		}
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}

	return &pulumirpc.RunResponse{}, nil
}

func (host *yamlLanguageHost) GetPluginInfo(ctx context.Context, req *pbempty.Empty) (*pulumirpc.PluginInfo, error) {
	return &pulumirpc.PluginInfo{
		Version: version.Version,
	}, nil
}

func (host *yamlLanguageHost) InstallDependencies(req *pulumirpc.InstallDependenciesRequest, server pulumirpc.LanguageRuntime_InstallDependenciesServer) error {
	// No dependencies to install for YAML
	return nil
}

// GetProgramDependencies returns the set of dependencies required by the program.
func (host *yamlLanguageHost) GetProgramDependencies(ctx context.Context, req *pulumirpc.GetProgramDependenciesRequest) (*pulumirpc.GetProgramDependenciesResponse, error) {
	return &pulumirpc.GetProgramDependenciesResponse{}, nil
}

// RuntimeOptionsPrompts returns a list of additional prompts to ask during `pulumi new`.
func (host *yamlLanguageHost) RuntimeOptionsPrompts(context.Context, *pulumirpc.RuntimeOptionsRequest) (*pulumirpc.RuntimeOptionsResponse, error) {
	return &pulumirpc.RuntimeOptionsResponse{}, nil
}

// About returns information about the runtime for this language.
func (host *yamlLanguageHost) About(ctx context.Context, req *pulumirpc.AboutRequest) (*pulumirpc.AboutResponse, error) {
	return &pulumirpc.AboutResponse{}, nil
}
