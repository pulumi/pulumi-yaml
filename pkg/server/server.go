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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pbempty "github.com/golang/protobuf/ptypes/empty"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/version"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/packages"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"

	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
)

type templateCacheEntry struct {
	template    *ast.TemplateDecl
	diagnostics syntax.Diagnostics
}

// yamlLanguageHost implements the LanguageRuntimeServer interface
// for use as an API endpoint.
type yamlLanguageHost struct {
	pulumirpc.LanguageRuntimeServer

	engineAddress string
	tracing       string
	compiler      string

	useRPCLoader  bool
	templateCache map[string]templateCacheEntry
}

func NewLanguageHost(engineAddress, tracing, compiler string, useRPCLoader bool) pulumirpc.LanguageRuntimeServer {
	return &yamlLanguageHost{
		engineAddress: engineAddress,
		tracing:       tracing,
		compiler:      compiler,

		useRPCLoader:  useRPCLoader,
		templateCache: make(map[string]templateCacheEntry),
	}
}

func (host *yamlLanguageHost) loadTemplate(directory string, compilerEnv []string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	// We can't cache comppiled templates because at the first point we call loadTemplate (in
	// GetRequiredPlugins) we don't have the compiler environment (with PULUMI_STACK etc) set.
	if entry, ok := host.templateCache[directory]; ok && host.compiler == "" {
		return entry.template, entry.diagnostics, nil
	}

	var template *ast.TemplateDecl
	var diags syntax.Diagnostics
	var err error
	if host.compiler == "" {
		template, diags, err = pulumiyaml.LoadDir(directory)
	} else {
		template, diags, err = pulumiyaml.LoadFromCompiler(host.compiler, directory, compilerEnv)
	}
	if err != nil {
		return nil, diags, err
	}
	if diags.HasErrors() {
		return nil, diags, nil
	}

	if host.compiler != "" {
		host.templateCache[directory] = templateCacheEntry{
			template:    template,
			diagnostics: diags,
		}
	}

	return template, diags, nil
}

// GetRequiredPlugins computes the complete set of anticipated plugins required by a program.
func (host *yamlLanguageHost) GetRequiredPlugins(ctx context.Context,
	req *pulumirpc.GetRequiredPluginsRequest,
) (*pulumirpc.GetRequiredPluginsResponse, error) {
	template, diags, err := host.loadTemplate(req.Info.ProgramDirectory, nil)
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

	projPath, err := workspace.DetectProjectPathFrom(req.Info.RootDirectory)
	if err != nil {
		return nil, err
	}
	proj, err := workspace.LoadProject(projPath)
	if err != nil {
		return nil, err
	}

	template, diags, err := host.loadTemplate(req.Info.ProgramDirectory, compilerEnv)
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

	// The yaml runtime is stateful and we need to change to the program directory before actually executing
	// the template.
	if pwd := req.Info.ProgramDirectory; pwd != "" {
		err := os.Chdir(pwd)
		if err != nil {
			return nil, err
		}
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

	// Because of async applies we may need the package loader to outlast the RunTemplate function. But by the
	// time RunWithContext returns we should be done with all async work.
	var loader pulumiyaml.PackageLoader
	if host.useRPCLoader {
		rpcLoader, err := schema.NewLoaderClient(req.LoaderTarget)
		if err != nil {
			return &pulumirpc.RunResponse{Error: err.Error()}, nil
		}
		loader = pulumiyaml.NewPackageLoaderFromSchemaLoader(
			schema.NewCachedLoader(rpcLoader))
	} else {
		loader, err = pulumiyaml.NewPackageLoader(proj.Plugins)
		if err != nil {
			return &pulumirpc.RunResponse{Error: err.Error()}, nil
		}
	}
	defer loader.Close()

	// Now instruct the Pulumi Go SDK to run the pulumi YAML interpreter.
	if err := pulumi.RunWithContext(pctx, func(ctx *pulumi.Context) error {
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
	// YAML doesn't _really_ have dependencies per-se but we can list all the "packages" that are referenced
	// in the program here. In the presesnce of parameterization this could differ to the set of plugins
	// reported by GetRequiredPlugins.

	template, diags, err := host.loadTemplate(req.Info.ProgramDirectory, nil)
	if err != nil {
		return nil, err
	}
	if diags.HasErrors() {
		return nil, diags
	}

	pkgs, pluginDiags := pulumiyaml.GetReferencedPackages(template)
	diags.Extend(pluginDiags...)
	if diags.HasErrors() {
		// We currently swallow the error to allow project config to evaluate
		// Specifically, if one sets a config key via the CLI but not within the `config` block
		// of their YAML program, it would error.
		return &pulumirpc.GetProgramDependenciesResponse{}, nil
	}
	var dependencies []*pulumirpc.DependencyInfo
	for _, pkg := range pkgs {
		name := pkg.Name
		version := pkg.Version
		if pkg.Parameterization != nil {
			name = pkg.Parameterization.Name
			version = pkg.Parameterization.Version
		}

		dependencies = append(dependencies, &pulumirpc.DependencyInfo{
			Name:    name,
			Version: version,
		})
	}
	return &pulumirpc.GetProgramDependenciesResponse{
		Dependencies: dependencies,
	}, nil
}

// RuntimeOptionsPrompts returns a list of additional prompts to ask during `pulumi new`.
func (host *yamlLanguageHost) RuntimeOptionsPrompts(context.Context, *pulumirpc.RuntimeOptionsRequest) (*pulumirpc.RuntimeOptionsResponse, error) {
	return &pulumirpc.RuntimeOptionsResponse{}, nil
}

// About returns information about the runtime for this language.
func (host *yamlLanguageHost) About(ctx context.Context, req *pulumirpc.AboutRequest) (*pulumirpc.AboutResponse, error) {
	return &pulumirpc.AboutResponse{}, nil
}

func (host *yamlLanguageHost) GenerateProject(
	ctx context.Context, req *pulumirpc.GenerateProjectRequest,
) (*pulumirpc.GenerateProjectResponse, error) {
	loader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, err
	}
	defer loader.Close()

	var extraOptions []pcl.BindOption
	if !req.Strict {
		extraOptions = append(extraOptions, pcl.NonStrictBindOptions()...)
	}

	program, diags, err := pcl.BindDirectory(req.SourceDirectory, schema.NewCachedLoader(loader), extraOptions...)
	if err != nil {
		return nil, err
	}
	if diags.HasErrors() {
		rpcDiagnostics := plugin.HclDiagnosticsToRPCDiagnostics(diags)

		return &pulumirpc.GenerateProjectResponse{
			Diagnostics: rpcDiagnostics,
		}, nil
	}

	var project workspace.Project
	if err := json.Unmarshal([]byte(req.Project), &project); err != nil {
		return nil, err
	}

	err = codegen.GenerateProject(req.TargetDirectory, project, program, req.LocalDependencies)
	if err != nil {
		return nil, err
	}

	rpcDiagnostics := plugin.HclDiagnosticsToRPCDiagnostics(diags)

	return &pulumirpc.GenerateProjectResponse{
		Diagnostics: rpcDiagnostics,
	}, nil
}

func (host *yamlLanguageHost) GenerateProgram(
	ctx context.Context, req *pulumirpc.GenerateProgramRequest,
) (*pulumirpc.GenerateProgramResponse, error) {
	loader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, err
	}
	defer loader.Close()

	parser := hclsyntax.NewParser()
	// Load all .pp files in the directory
	for path, contents := range req.Source {
		err = parser.ParseFile(strings.NewReader(contents), path)
		if err != nil {
			return nil, err
		}
		diags := parser.Diagnostics
		if diags.HasErrors() {
			return nil, diags
		}
	}

	bindOptions := []pcl.BindOption{
		pcl.Loader(schema.NewCachedLoader(loader)),
		pcl.PreferOutputVersionedInvokes,
	}

	if !req.Strict {
		bindOptions = append(bindOptions, pcl.NonStrictBindOptions()...)
	}

	program, diags, err := pcl.BindProgram(parser.Files, bindOptions...)
	if err != nil {
		return nil, err
	}

	rpcDiagnostics := plugin.HclDiagnosticsToRPCDiagnostics(diags)
	if diags.HasErrors() {
		return &pulumirpc.GenerateProgramResponse{
			Diagnostics: rpcDiagnostics,
		}, nil
	}
	if program == nil {
		return nil, fmt.Errorf("internal error program was nil")
	}

	files, diags, err := codegen.GenerateProgram(program)
	if err != nil {
		return nil, err
	}
	rpcDiagnostics = append(rpcDiagnostics, plugin.HclDiagnosticsToRPCDiagnostics(diags)...)

	return &pulumirpc.GenerateProgramResponse{
		Source:      files,
		Diagnostics: rpcDiagnostics,
	}, nil
}

func (host *yamlLanguageHost) GeneratePackage(ctx context.Context, req *pulumirpc.GeneratePackageRequest) (*pulumirpc.GeneratePackageResponse, error) {
	// YAML doesn't generally have "SDKs" per-se but we can write out a "lock file" for a given package name and
	// version, and if using a parameterized package this is necessary so that we have somewhere to save the parameter
	// value.

	if len(req.ExtraFiles) > 0 {
		return nil, errors.New("overlays are not supported for YAML")
	}

	loader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, err
	}

	var spec schema.PackageSpec
	err = json.Unmarshal([]byte(req.Schema), &spec)
	if err != nil {
		return nil, err
	}

	pkg, diags, err := schema.BindSpec(spec, loader)
	if err != nil {
		return nil, err
	}
	rpcDiagnostics := plugin.HclDiagnosticsToRPCDiagnostics(diags)
	if diags.HasErrors() {
		return &pulumirpc.GeneratePackageResponse{
			Diagnostics: rpcDiagnostics,
		}, nil
	}

	// Generate the a package lock file in the given directory. This is just a simple YAML file that contains the name,
	// version, and any parameter values.
	lock := packages.PackageDecl{
		PackageDeclarationVersion: 1,
	}
	// The format of the lock file differs based on if this is a parameterized package or not.
	if pkg.Parameterization == nil {
		lock.Name = pkg.Name
		if pkg.Version != nil {
			lock.Version = pkg.Version.String()
		}
		lock.DownloadURL = pkg.PluginDownloadURL
	} else {
		lock.Name = pkg.Parameterization.BaseProvider.Name
		lock.Version = pkg.Parameterization.BaseProvider.Version.String()
		lock.DownloadURL = pkg.PluginDownloadURL
		if pkg.Version == nil {
			return nil, errors.New("parameterized package must have a version")
		}
		lock.Parameterization = &packages.ParameterizationDecl{
			Name:    pkg.Name,
			Version: pkg.Version.String(),
		}
		lock.Parameterization.SetValue(pkg.Parameterization.Parameter)
	}

	// Write out a yaml file for this package
	var version string
	if pkg.Version != nil {
		version = fmt.Sprintf("-%s", pkg.Version.String())
	}
	dest := filepath.Join(req.Directory, fmt.Sprintf("%s%s.yaml", pkg.Name, version))

	data, err := yaml.Marshal(lock)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(req.Directory, 0o700)
	if err != nil {
		return nil, fmt.Errorf("could not create output directory %s: %w", req.Directory, err)
	}

	err = os.WriteFile(dest, data, 0o600)
	if err != nil {
		return nil, fmt.Errorf("could not write output file %s: %w", dest, err)
	}

	return &pulumirpc.GeneratePackageResponse{
		Diagnostics: rpcDiagnostics,
	}, nil
}

func (host *yamlLanguageHost) Pack(ctx context.Context, req *pulumirpc.PackRequest) (*pulumirpc.PackResponse, error) {
	// Yaml "SDKs" are just files, we can just copy the file
	if err := os.MkdirAll(req.DestinationDirectory, 0700); err != nil {
		return nil, err
	}

	files, err := os.ReadDir(req.PackageDirectory)
	if err != nil {
		return nil, fmt.Errorf("reading package directory: %w", err)
	}

	copyFile := func(src, dst string) error {
		srcFile, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("opening %s: %w", src, err)
		}
		defer srcFile.Close()
		dstFile, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("creating %s: %w", dst, err)
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("copying %s to %s: %w", src, dst, err)
		}
		return nil
	}

	// We only expect one file in the package directory
	var single string
	for _, file := range files {
		if single != "" {
			return nil, fmt.Errorf("multiple files in package directory %s: %s and %s", req.PackageDirectory, single, file.Name())
		}
		single = file.Name()
	}

	src := filepath.Join(req.PackageDirectory, single)
	dst := filepath.Join(req.DestinationDirectory, single)
	if err := copyFile(src, dst); err != nil {
		return nil, fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}

	return &pulumirpc.PackResponse{
		ArtifactPath: dst,
	}, nil
}
