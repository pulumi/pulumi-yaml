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
	"slices"
	"strconv"
	"strings"
	"time"

	pbempty "github.com/golang/protobuf/ptypes/empty"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	providersdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/packages"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/version"

	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
)

type templateCacheEntry struct {
	template    *ast.TemplateDecl
	diagnostics syntax.Diagnostics
}

// yamlLanguageHost implements the LanguageRuntimeServer interface
// for use as an API endpoint.
type yamlLanguageHost struct {
	pulumirpc.UnimplementedLanguageRuntimeServer

	engineAddress string
	tracing       string

	templateCache map[string]templateCacheEntry
}

func NewLanguageHost(engineAddress, tracing string) pulumirpc.LanguageRuntimeServer {
	return &yamlLanguageHost{
		engineAddress: engineAddress,
		tracing:       tracing,
		templateCache: make(map[string]templateCacheEntry),
	}
}

func (host *yamlLanguageHost) loadPluginTemplate(directory string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	template, diags, err := pulumiyaml.LoadPluginTemplate(directory)
	if err != nil {
		return nil, diags, err
	}
	if diags.HasErrors() {
		return nil, diags, nil
	}

	return template, diags, nil
}

func (host *yamlLanguageHost) loadTemplate(compiler, directory string, compilerEnv []string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	// We can't cache comppiled templates because at the first point we call loadTemplate (in
	// GetRequiredPackages) we don't have the compiler environment (with PULUMI_STACK etc) set.
	if entry, ok := host.templateCache[directory]; ok && compiler == "" {
		return entry.template, entry.diagnostics, nil
	}

	var template *ast.TemplateDecl
	var diags syntax.Diagnostics
	var err error
	if compiler == "" {
		template, diags, err = pulumiyaml.LoadDir(directory)
	} else {
		template, diags, err = pulumiyaml.LoadFromCompiler(compiler, directory, compilerEnv)
	}
	if err != nil {
		return nil, diags, err
	}
	if diags.HasErrors() {
		return nil, diags, nil
	}

	if compiler != "" {
		host.templateCache[directory] = templateCacheEntry{
			template:    template,
			diagnostics: diags,
		}
	}

	return template, diags, nil
}

func parseCompiler(options map[string]interface{}) (string, error) {
	if compiler, ok := options["compiler"]; ok {
		if compiler, ok := compiler.(string); ok {
			return compiler, nil
		}
		return "", errors.New("binary option must be a string")
	}

	return "", nil
}

// GetRequiredPackages computes the complete set of anticipated packages required by a program.
func (host *yamlLanguageHost) GetRequiredPackages(ctx context.Context,
	req *pulumirpc.GetRequiredPackagesRequest,
) (*pulumirpc.GetRequiredPackagesResponse, error) {
	compiler, err := parseCompiler(req.Info.Options.AsMap())
	if err != nil {
		return nil, err
	}

	template, diags, err := host.loadTemplate(compiler, req.Info.ProgramDirectory, nil)
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
		return &pulumirpc.GetRequiredPackagesResponse{}, nil
	}
	var packages []*pulumirpc.PackageDependency
	for _, pkg := range pkgs {
		var parameterization *pulumirpc.PackageParameterization
		if pkg.Parameterization != nil {
			value, err := pkg.Parameterization.GetValue()
			if err != nil {
				return nil, fmt.Errorf("decoding parameter value for package %s: %w", pkg.Name, err)
			}
			parameterization = &pulumirpc.PackageParameterization{
				Name:    pkg.Parameterization.Name,
				Version: pkg.Parameterization.Version,
				Value:   value,
			}
		}

		packages = append(packages, &pulumirpc.PackageDependency{
			Kind:             string(apitype.ResourcePlugin),
			Name:             pkg.Name,
			Version:          pkg.Version,
			Server:           pkg.DownloadURL,
			Parameterization: parameterization,
		})
	}
	return &pulumirpc.GetRequiredPackagesResponse{
		Packages: packages,
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
		fmt.Sprintf(`PULUMI_ROOT_DIRECTORY=%s`, req.Info.RootDirectory),
	}

	compiler, err := parseCompiler(req.Info.Options.AsMap())
	if err != nil {
		return nil, err
	}

	template, diags, err := host.loadTemplate(compiler, req.Info.ProgramDirectory, compilerEnv)
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

	// Translate the string based config map into property values for the rest of the system.
	confPropMap := resource.PropertyMap{}
	for k, v := range configValue {
		var value resource.PropertyValue
		var jsonValue interface{}

		// scalar config values are represented as their go literals, while arrays and objects
		// are represented as JSON.
		if len(v) > 1 && strings.HasPrefix(v, "0") && !strings.HasPrefix(v, "0.") {
			// If the value starts with 0 or symbol we assume it is a string, not a number. This is to avoid cases like
			// "01234" in config files being turned into numbers. But just "0" or "0.0" are still valid numbers.
			value = resource.NewStringProperty(v)
		} else if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			value = resource.NewNumberProperty(float64(i))
		} else if b, err := strconv.ParseBool(v); err == nil {
			value = resource.NewBoolProperty(b)
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			value = resource.NewNumberProperty(float64(f))
		} else if err := json.Unmarshal([]byte(v), &jsonValue); err == nil {
			value = resource.NewPropertyValue(jsonValue)
		} else {
			value = resource.NewStringProperty(v)
		}

		if slices.Contains(req.ConfigSecretKeys, k) {
			value = resource.MakeSecret(value)
		}

		confPropMap[resource.PropertyKey(k)] = value
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
		Project:          req.GetProject(),
		RootDirectory:    req.Info.RootDirectory,
		Stack:            req.GetStack(),
		Config:           req.GetConfig(),
		ConfigSecretKeys: req.GetConfigSecretKeys(),
		Organization:     req.Organization,
		Parallel:         req.GetParallel(),
		DryRun:           req.GetDryRun(),
		MonitorAddr:      req.GetMonitorAddress(),
		EngineAddr:       host.engineAddress,
	})
	if err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}
	defer pctx.Close()

	// Because of async applies we may need the package loader to outlast the RunTemplate function. But by the
	// time RunWithContext returns we should be done with all async work.
	rpcLoader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}
	loader := pulumiyaml.NewPackageLoaderFromSchemaLoader(
		schema.NewCachedLoader(rpcLoader))
	defer loader.Close()

	// Now instruct the Pulumi Go SDK to run the pulumi YAML interpreter.
	if err := pulumi.RunWithContext(pctx, func(ctx *pulumi.Context) error {
		// Now "evaluate" the template.
		return pulumiyaml.RunTemplate(pctx, template, confPropMap, loader)
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

func (host *yamlLanguageHost) RunPlugin(
	req *pulumirpc.RunPluginRequest, server pulumirpc.LanguageRuntime_RunPluginServer,
) error {
	logging.V(5).Infof("Attempting to run yaml plugin in %s", req.Info.ProgramDirectory)
	ctx := context.Background()

	closer, stdout, stderr, err := rpcutil.MakeRunPluginStreams(server, false)
	if err != nil {
		return err
	}
	defer closer.Close()

	template, diags, err := host.loadPluginTemplate(req.Info.ProgramDirectory)
	if err != nil {
		return err
	}

	diagWriter := template.NewDiagnosticWriter(stderr, 0, true)
	if len(diags) != 0 {
		err := diagWriter.WriteDiagnostics(diags.HCL())
		if err != nil {
			return err
		}
	}
	if diags.HasErrors() {
		return errors.New("failed to load template")
	}

	// Use the Pulumi Go SDK to create an execution context and to interact with the engine.
	// This encapsulates a fair bit of the boilerplate otherwise needed to do RPCs, etc.
	pctx, err := pulumi.NewContext(ctx, pulumi.RunInfo{
		EngineAddr: host.engineAddress,
	})
	if err != nil {
		return err
	}
	defer pctx.Close()

	rpcLoader, err := schema.NewLoaderClient(host.engineAddress)
	if err != nil {
		return err
	}

	// Because of async applies we may need the package loader to outlast the RunTemplate function. But by the
	// time RunWithContext returns we should be done with all async work.
	loader := pulumiyaml.NewPackageLoaderFromSchemaLoader(schema.NewCachedLoader(rpcLoader))
	defer loader.Close()

	schema, err := template.GenerateSchema()
	if err != nil {
		return err
	}

	jsonSchema, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	var cancelChannel chan bool
	providerHost, err := provider.NewHostClient(host.engineAddress)
	if err != nil {
		return fmt.Errorf("fatal: could not connect to host RPC: %w", err)
	}

	// If we have a host cancel our cancellation context if it fails the healthcheck
	ctx, cancel := context.WithCancel(context.Background())
	// map the context Done channel to the rpcutil boolean cancel channel
	cancelChannel = make(chan bool)
	go func() {
		<-ctx.Done()
		close(cancelChannel)
	}()
	err = rpcutil.Healthcheck(ctx, host.engineAddress, 5*time.Minute, cancel)
	if err != nil {
		return fmt.Errorf("could not start health check host RPC server: %w", err)
	}

	prov := &componentProvider{
		host:   providerHost.EngineConn(),
		name:   template.Name.Value,
		schema: jsonSchema,
		construct: func(ctx *pulumi.Context, typ, name string, inputs providersdk.ConstructInputs,
			options pulumi.ResourceOption,
		) (*providersdk.ConstructResult, error) {
			m, err := inputs.Map()
			if err != nil {
				return nil, err
			}
			urn, state, err := pulumiyaml.RunComponentTemplate(ctx, typ, name, options, template, m, loader)
			if err != nil {
				return nil, err
			}
			return &providersdk.ConstructResult{
				URN:   urn,
				State: state,
			}, nil
		},
	}

	// Fire up a gRPC server, letting the kernel choose a free port for us.
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancelChannel,
		Init: func(srv *grpc.Server) error {
			pulumirpc.RegisterResourceProviderServer(srv, prov)
			return nil
		},
		Options: rpcutil.OpenTracingServerInterceptorOptions(nil),
	})
	if err != nil {
		return fmt.Errorf("fatal: %w", err)
	}

	// The resource provider protocol requires that we now write out the port we have chosen to listen on.
	fmt.Fprintf(stdout, "%d\n", handle.Port)

	// Finally, wait for the server to stop serving.
	if err := <-handle.Done; err != nil {
		return fmt.Errorf("fatal: %w", err)
	}

	return nil
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
	// in the program here.
	compiler, err := parseCompiler(req.Info.Options.AsMap())
	if err != nil {
		return nil, err
	}

	template, diags, err := host.loadTemplate(compiler, req.Info.ProgramDirectory, nil)
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

	pkg, diags, err := schema.BindSpec(spec, loader, schema.ValidationOptions{
		AllowDanglingReferences: true,
	})
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
	if err := os.MkdirAll(req.DestinationDirectory, 0o700); err != nil {
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

func (host *yamlLanguageHost) Link(context.Context, *pulumirpc.LinkRequest) (*pulumirpc.LinkResponse, error) {
	// YAML doesn't need to do anything to link in a package.
	//
	// We still implement Link so the engine knows that we have done all we need to do.
	return &pulumirpc.LinkResponse{}, nil
}
