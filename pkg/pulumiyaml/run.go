// Copyright 2022-2025, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/shlex"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	ctypes "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/config"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/packages"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
)

// MainTemplate is the assumed name of the JSON template file.
// TODO: would be nice to permit multiple files, but we'd need to know which is "main", and there's
// no notion of "import" so we'd need to be a bit more clever. Might be nice to mimic e.g. Kustomize.
// One idea is to hijack Pulumi.yaml's "main" directive and then just globally toposort the rest.
const MainTemplate = "Main"

func LoadFromCompiler(compiler string, workingDirectory string, env []string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	var diags syntax.Diagnostics
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	argv, err := shlex.Split(compiler)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing compiler argument: %v", err)
	}

	name := argv[0]
	cmd := exec.Command(name, argv[1:]...)
	cmd.Dir = workingDirectory
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(env, os.Environ()...)
	for _, duplicate := range conflictingEnvVars(cmd.Env) {
		diags = append(diags, syntax.Warning(nil, fmt.Sprintf("environment variable %v is already set: compiler %v will not override conflicting environment variables", duplicate, name), ""))
	}

	err = cmd.Run()
	if err != nil {
		return nil, nil, fmt.Errorf("error running compiler %v: %v, stderr follows: %v", name, err, stderr.String())
	}
	if stdout.Len() != 0 {
		diags = append(diags, syntax.Warning(nil, fmt.Sprintf("compiler %v warnings: %v", name, stdout.String()), ""))
	}

	templateStr := stdout.String()
	template, tdiags, err := LoadYAMLBytes(fmt.Sprintf("<stdout from compiler %v>", name), []byte(templateStr))
	diags.Extend(tdiags...)
	if err != nil {
		return nil, diags, err
	}

	if template.Name == nil {
		uncompiledTemplate, _, err := LoadDir(workingDirectory)
		if err != nil || uncompiledTemplate.Name == nil {
			return nil, diags, errors.New("compiler did not produce a valid template")
		}
		template.Name = uncompiledTemplate.Name
	}

	sdks, err := packages.SearchPackageDecls(workingDirectory)
	if err != nil {
		diags.Extend(syntax.Error(nil, err.Error(), ""))
	}
	template.Sdks = sdks

	return template, tdiags, err
}

func conflictingEnvVars(env []string) []string {
	envMap := make(map[string]uint)
	var duplicates []string
	for _, e := range env {
		key := strings.Split(e, "=")[0]
		if cnt, exists := envMap[key]; exists && cnt <= 1 {
			duplicates = append(duplicates, key)
		}

		envMap[key]++
	}

	return duplicates
}

// Load a template from the given directory.
func LoadDir(directory string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	// Read in the template file - search first for Main.json, then Main.yaml, then Pulumi.yaml.
	// The last of these will actually read the proram from the same Pulumi.yaml project file used by
	// Pulumi CLI, which now plays double duty, and allows a Pulumi deployment that uses a single file.
	var filename string
	var bs []byte
	if b, err := os.ReadFile(filepath.Join(directory, MainTemplate+".json")); err == nil {
		filename, bs = MainTemplate+".json", b
	} else if b, err := os.ReadFile(filepath.Join(directory, MainTemplate+".yaml")); err == nil {
		filename, bs = MainTemplate+".yaml", b
	} else if b, err := os.ReadFile(filepath.Join(directory, "Pulumi.yaml")); err == nil {
		filename, bs = "Pulumi.yaml", b
	} else {
		return nil, nil, fmt.Errorf("reading template %s: %w", MainTemplate, err)
	}

	template, diags, err := LoadYAMLBytes(filename, bs)
	if err != nil {
		return nil, diags, err
	}

	if diags.HasErrors() {
		return nil, diags, diags
	}

	sdks, err := packages.SearchPackageDecls(directory)
	if err != nil {
		diags.Extend(syntax.Error(nil, err.Error(), ""))
	}
	template.Sdks = sdks

	return template, diags, nil
}

// Load a plugin template from the given directory.
func LoadPluginTemplate(directory string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	// Get all yaml files in the directory, load them and merge them into a single template.
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, err
	}
	var template *ast.TemplateDecl
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".yaml" {
			continue
		}

		f, err := os.Open(filepath.Join(directory, file.Name()))
		if err != nil {
			return nil, nil, err
		}
		t, diags, err := LoadYAML(file.Name(), f)
		if err != nil {
			return nil, diags, err
		}
		if diags.HasErrors() {
			return nil, diags, diags
		}
		if template == nil {
			template = t
		} else {
			if t == nil {
				diags.Extend(syntax.Error(nil, "failed to load template", ""))
				continue
			}
			if len(t.Configuration.Entries) > 0 {
				diags.Extend(syntax.Error(nil, "PulumiPlugin.yaml: root-level `configuration` is not supported in plugins", ""))
			}
			if len(t.Config.Entries) > 0 {
				diags.Extend(syntax.Error(nil, "PulumiPlugin.yaml: root-level `config` is not supported in plugins", ""))
			}
			if len(t.Variables.Entries) > 0 {
				diags.Extend(syntax.Error(nil, "PulumiPlugin.yaml: root-level `variables` field is not supported in plugins.", ""))
			}
			if len(t.Resources.Entries) > 0 {
				diags.Extend(syntax.Error(nil, "PulumiPlugin.yaml: root-level `resources` field is not supported in plugins.", ""))
			}
			if len(t.Outputs.Entries) > 0 {
				diags.Extend(syntax.Error(nil, "PulumiPlugin.yaml: root-level `outputs` field is not supported in plugins.", ""))
			}

			err = template.Merge(t)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	componentNames := make(map[string]bool)
	for _, entry := range template.Components.Entries {
		if _, ok := componentNames[entry.Value.Name.Value]; ok {
			return nil, nil, fmt.Errorf("duplicate component name %s", entry.Value.Name.Value)
		}
		componentNames[entry.Value.Name.Value] = true
	}
	sdks, err := packages.SearchPackageDecls(directory)
	if err != nil {
		return nil, nil, err
	}
	template.Sdks = sdks

	return template, nil, nil
}

// LoadYAML decodes a YAML template from an io.Reader.
func LoadYAML(filename string, r io.Reader) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	return LoadYAMLBytes(filename, bytes)
}

// LoadYAMLBytes decodes a YAML template from a byte array.
func LoadYAMLBytes(filename string, source []byte) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	var diags syntax.Diagnostics

	syn, sdiags := encoding.DecodeYAML(filename, yaml.NewDecoder(bytes.NewReader(source)), TagDecoder)
	diags.Extend(sdiags...)
	if sdiags.HasErrors() {
		return nil, diags, nil
	}

	t, tdiags := ast.ParseTemplate(source, syn)
	diags.Extend(tdiags...)
	if tdiags.HasErrors() {
		return nil, diags, nil
	}
	if t.Configuration.Entries != nil {
		diags = append(diags, syntax.Warning(nil, "Pulumi.yaml: root-level `configuration` field is deprecated; please use `config` instead.", ""))
	}

	return t, diags, nil
}

// LoadTemplate decodes a Template value into a YAML template.
func LoadTemplate(t *Template) (*ast.TemplateDecl, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	syn, sdiags := encoding.DecodeValue(t)
	diags.Extend(sdiags...)
	if sdiags.HasErrors() {
		return nil, diags
	}

	td, tdiags := ast.ParseTemplate(nil, syn)
	diags.Extend(tdiags...)

	return td, diags
}

func HasDiagnostics(err error) (syntax.Diagnostics, bool) {
	if err == nil {
		return nil, false
	}

	switch err := err.(type) {
	case syntax.Diagnostics:
		if len(err) > 0 {
			return err, true
		}
		return nil, false
	case *multierror.Error:
		var diags syntax.Diagnostics
		var has bool
		for _, err := range err.Errors {
			if ediags, ok := HasDiagnostics(err); ok {
				diags.Extend(ediags...)
				has = true
			}
		}
		return diags, has
	default:
		var diags syntax.Diagnostics
		return diags, errors.As(err, &diags)
	}
}

// validateResources does some basic validation of each resource to provide
// error messages for any missing required fields.
func (r *Runner) validateResources() {
	for _, resource := range r.t.GetResources().Entries {
		v := resource.Value
		if v.Type == nil {
			r.sdiags.Extend(syntax.NodeError(
				v.Syntax(),
				fmt.Sprintf("Required field 'type' is missing on resource \"%s\"", resource.Key.Value), ""))
		}
	}
}

// Set default providers for resources and invokes.
//
// This function communicates errors by appending to the internal diags field of `r`.
// It is the responsibility of the caller to verify that no err diags were appended if
// that should prevent proceeding.
func (r *Runner) setDefaultProviders() {
	defaultProviderInfoMap := make(map[string]*providerInfo)
	for _, resource := range r.t.GetResources().Entries {
		v := resource.Value
		// check if this is a provider resource
		if v.Type != nil && strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
			pkgName := strings.Split(v.Type.Value, "pulumi:providers:")[1]
			// check if it's set as a default provider
			if v.DefaultProvider != nil && v.DefaultProvider.Value {
				defaultProviderInfoMap[pkgName] = &providerInfo{
					version:           v.Options.Version,
					pluginDownloadURL: v.Options.PluginDownloadURL,
					providerName:      resource.Key,
				}
			}
		} else if v.DefaultProvider != nil {
			r.sdiags.Extend(syntax.NodeError(
				v.DefaultProvider.Syntax(),
				"cannot set defaultProvider on non-provider resource", ""))
		}
	}

	// Set roots
	diags := r.Run(walker{
		VisitResource: func(r *Runner, node resourceNode) bool {
			k, v := node.Key.Value, node.Value
			ctx := r.newContext(node)

			if v.Type == nil {
				return false
			}

			if strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
				return true
			}
			pkgName := strings.Split(v.Type.Value, ":")[0]

			defaultProviderInfo, ok := defaultProviderInfoMap[pkgName]
			if !ok {
				return true
			}

			if v.Options.Provider == nil {
				if v.Options.Version != nil && v.Options.Version.Value != defaultProviderInfo.version.Value {
					ctx.addErrDiag(node.Key.Syntax().Syntax().Range(),
						"Version conflicts with the default provider version",
						fmt.Sprintf("Try removing this option on resource \"%s\"", k))
				}
				if v.Options.PluginDownloadURL != nil && v.Options.PluginDownloadURL.Value != defaultProviderInfo.pluginDownloadURL.Value {
					ctx.addErrDiag(node.Key.Syntax().Syntax().Range(),
						"PluginDownloadURL conflicts with the default provider URL",
						fmt.Sprintf("Try removing this option on resource \"%s\"", k))
				}

				expr, diags := ast.VariableSubstitution(defaultProviderInfo.providerName.Value)
				if diags.HasErrors() {
					r.sdiags.diags = append(r.sdiags.diags, diags...)
					return false
				}

				v.Options.Provider = expr
				v.Options.Version = defaultProviderInfo.version
			}
			return true
		},
		VisitExpr: func(ec *evalContext, e ast.Expr) bool {
			return true
		},
		VisitVariable: func(r *Runner, node variableNode) bool {
			k, v := node.Key.Value, node.Value
			ctx := r.newContext(node)

			switch t := v.(type) {
			case *ast.InvokeExpr:
				pkgName := strings.Split(t.Token.Value, ":")[0]
				if _, ok := defaultProviderInfoMap[pkgName]; !ok {
					return true
				}
				defaultProviderInfo := defaultProviderInfoMap[pkgName]

				if t.CallOpts.Provider == nil {
					if t.CallOpts.Version != nil {
						ctx.addErrDiag(node.Key.Syntax().Syntax().Range(),
							"Version conflicts with the default provider version",
							fmt.Sprintf("Try removing this option on resource \"%s\"", k))
					}
					if t.CallOpts.PluginDownloadURL != nil {
						ctx.addErrDiag(node.Key.Syntax().Syntax().Range(),
							"PluginDownloadURL conflicts with the default provider URL",
							fmt.Sprintf("Try removing this option on resource \"%s\"", k))
					}

					expr, diags := ast.VariableSubstitution(defaultProviderInfo.providerName.Value)
					if diags.HasErrors() {
						r.sdiags.diags = append(r.sdiags.diags, diags...)
						return false
					}
					t.CallOpts.Provider = expr
					t.CallOpts.Version = defaultProviderInfo.version
				}
			}
			return true
		},
		VisitConfig: func(r *Runner, node configNode) bool {
			return true
		},
		VisitOutput: func(r *Runner, node ast.PropertyMapEntry) bool {
			return true
		},
	})

	// This function communicates errors by appending to the internal diags field of `r`.
	// It is the responsibility of the caller to verify that no err diags were appended if
	// that should prevent proceeding.
	contract.IgnoreError(diags)
}

// Set the runner's package descriptors from the templates package decls.
func (r *Runner) setPackageDesciptors() error {
	// Register package refs for all packages we know upfront
	packageDescriptors, err := packages.ToPackageDescriptors(r.t.GetSdks())
	if err != nil {
		return err
	}
	r.packageDescriptors = packageDescriptors
	return nil
}

// PrepareTemplate prepares a template for converting or running
func PrepareTemplate(t *ast.TemplateDecl, r *Runner, loader PackageLoader) (*Runner, syntax.Diagnostics, error) {
	// If running a template also, we need to pass a runner through, since setting intermediates
	// requires config via the pulumi Context
	if r == nil {
		r = newRunner(t, loader)
	}

	// We are preemptively calling r.setIntermediates. We are forcing tolerating missing
	// nodes, ensuring the process can continue even for invalid templates. Diags will
	// still be reported normally.
	//
	// r.setDefaultProviders uses r.setIntermediates, so this line need to precede calls
	// to r.setDefaultProviders.
	r.setIntermediates("", nil, true /*force*/)

	// do some basic validation of each resource
	r.validateResources()

	// runner hooks up default providers
	r.setDefaultProviders()

	// fill in the package descriptors from the templates package decls
	err := r.setPackageDesciptors()
	if err != nil {
		return nil, nil, err
	}

	// runner type checks nodes
	_, diags := TypeCheck(r)
	return r, diags, nil
}

// RunTemplate runs the programEvaluator against a template using the given request/settings.
func RunTemplate(ctx *pulumi.Context, t *ast.TemplateDecl, configPropertyMap resource.PropertyMap, loader PackageLoader) error {
	if len(t.Components.Entries) > 0 {
		return errors.New("components are only supported in plugins, not in programs")
	}
	if t.Namespace != nil {
		return errors.New("namespace is only supported in component plugins")
	}
	r := newRunner(t, loader)
	r.setIntermediates(ctx.Project(), configPropertyMap, false)
	if r.sdiags.HasErrors() {
		return &r.sdiags
	}

	r, diags, err := PrepareTemplate(t, r, loader)
	if diags.HasErrors() {
		return diags
	}
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return diags
	}

	// runtime evaluation here
	diags.Extend(r.Evaluate(ctx)...)
	if diags.HasErrors() {
		return diags
	}
	return nil
}

func RunComponentTemplate(ctx *pulumi.Context,
	typ, name string, options pulumi.ResourceOption,
	t *ast.TemplateDecl, inputs pulumi.Map, loader PackageLoader,
) (pulumi.URNOutput, pulumi.Map, error) {
	var templ ast.Template
	typSplit := strings.Split(typ, ":")
	if len(typSplit) != 3 {
		return pulumi.URNOutput{}, nil, errors.New("invalid component type")
	}
	for _, comp := range t.Components.Entries {
		if comp.Key.Value == typSplit[2] {
			templ = comp.Value
			break
		}
	}
	runner := newRunner(templ, loader)

	_, diags := TypeCheck(runner)
	if diags.HasErrors() {
		return pulumi.URNOutput{}, nil, diags
	}

	packageRefs, diags := findPackageRefs(ctx, runner)
	if diags != nil {
		return pulumi.URNOutput{}, nil, errors.New(diags.Error())
	}

	component := &componentEvaluator{
		inputs:  inputs,
		outputs: pulumi.Map{},
		evaluator: &programEvaluator{
			evalContext: runner.newContext(t),
			pulumiCtx:   ctx,
			packageRefs: packageRefs,
		},
	}

	if err := ctx.RegisterComponentResource(typ, name, component, options); err != nil {
		return pulumi.URNOutput{}, nil, err
	}

	diags.Extend(runner.Run(component)...)
	if diags.HasErrors() {
		return pulumi.URNOutput{}, nil, diags
	}
	if err := ctx.RegisterResourceOutputs(component, component.outputs); err != nil {
		return pulumi.URNOutput{}, nil, err
	}
	return component.URN(), component.outputs, nil
}

type componentEvaluator struct {
	pulumi.ResourceState

	inputs  pulumi.Map
	outputs pulumi.Map

	evaluator *programEvaluator
}

func (m *componentEvaluator) EvalConfig(r *Runner, node configNode) bool {
	k := node.key().Value
	v, ok := m.inputs[k]
	if ok {
		r.config[k] = v
		return true
	}
	return m.evaluator.EvalConfig(r, node)
}

func (m *componentEvaluator) EvalVariable(r *Runner, node variableNode) bool {
	return m.evaluator.EvalVariable(r, node)
}

func (m *componentEvaluator) EvalResource(r *Runner, node resourceNode) bool {
	ctx := r.newContext(node)
	res, ok := m.evaluator.registerResourceWithParent(node, m)
	if !ok {
		msg := fmt.Sprintf("Error registering resource [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := m.evaluator.pulumiCtx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
		return false
	}
	r.resources[node.Key.Value] = res
	return true
}

func (m *componentEvaluator) EvalOutput(r *Runner, node ast.PropertyMapEntry) bool {
	ctx := r.newContext(node)
	out, ok := m.evaluator.registerOutput(node)
	if !ok {
		msg := fmt.Sprintf("Error registering output [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := m.evaluator.pulumiCtx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else {
		m.outputs[node.Key.Value] = out
	}
	return true
}

func (m *componentEvaluator) EvalMissing(r *Runner, node missingNode) bool {
	return m.evaluator.EvalMissing(r, node)
}

type syncDiags struct {
	diags syntax.Diagnostics
	mutex sync.Mutex
}

func (d *syncDiags) Extend(diags ...*syntax.Diagnostic) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.diags.Extend(diags...)
}

func (d *syncDiags) Error() string {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.diags.Error()
}

func (d *syncDiags) HasErrors() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	return d.diags.HasErrors()
}

type providerInfo struct {
	version           *ast.StringExpr
	pluginDownloadURL *ast.StringExpr
	providerName      *ast.StringExpr
}

type Runner struct {
	t                  ast.Template
	packageDescriptors map[tokens.Package]*schema.PackageDescriptor

	pkgLoader PackageLoader
	config    map[string]interface{}
	variables map[string]interface{}
	resources map[string]lateboundResource
	stackRefs map[string]*pulumi.StackReference

	cwd string

	sdiags syncDiags

	// Used to store sorted nodes. A non `nil` value indicates that the runner
	// is already setup for running.
	intermediates []graphNode
}

type evalContext struct {
	*Runner

	root   interface{}
	sdiags syncDiags
}

func (ctx *evalContext) addWarnDiag(rng *hcl.Range, summary string, detail string) {
	ctx.sdiags.diags.Extend(syntax.Warning(rng, summary, detail))
	ctx.Runner.sdiags.diags.Extend(syntax.Warning(rng, summary, detail))
}

func (ctx *evalContext) addErrDiag(rng *hcl.Range, summary string, detail string) {
	ctx.sdiags.diags.Extend(syntax.Error(rng, summary, detail))
	ctx.Runner.sdiags.diags.Extend(syntax.Error(rng, summary, detail))
}

func (ctx *evalContext) error(expr ast.Expr, summary string) (interface{}, bool) {
	diag := ast.ExprError(expr, summary, "")
	ctx.sdiags.Extend(diag)
	ctx.Runner.sdiags.Extend(diag)
	return nil, false
}

func (ctx *evalContext) errorf(expr ast.Expr, format string, a ...interface{}) (interface{}, bool) {
	return ctx.error(expr, fmt.Sprintf(format, a...))
}

func (r *Runner) newContext(root interface{}) *evalContext {
	ctx := &evalContext{
		Runner: r,
		root:   root,
		sdiags: syncDiags{},
	}

	return ctx
}

// lateboundResource is an interface shared by lateboundCustomResourceState and
// lateboundProviderResourceState so that both normal and provider resources can be
// created and managed as part of a deployment.
type lateboundResource interface {
	GetOutput(k string) pulumi.Output
	GetOutputs() pulumi.Output
	CustomResource() *pulumi.CustomResourceState
	ProviderResource() *pulumi.ProviderResourceState
	GetRawOutputs() pulumi.Output
	GetResourceSchema() *schema.Resource
}

// lateboundCustomResourceState is a resource state that stores all computed outputs into a single
// MapOutput, so that we can access any output that was provided by the Pulumi engine without knowing
// up front the shape of the expected outputs.
type lateboundCustomResourceState struct {
	pulumi.CustomResourceState
	name           string
	Outputs        pulumi.MapOutput `pulumi:""`
	resourceSchema *schema.Resource
}

// GetOutputs returns the resource's outputs.
func (st *lateboundCustomResourceState) GetOutputs() pulumi.Output {
	return st.Outputs
}

// GetOutput returns the named output of the resource.
func (st *lateboundCustomResourceState) GetOutput(k string) pulumi.Output {
	return st.Outputs.ApplyT(func(outputs map[string]interface{}) (interface{}, error) {
		out, ok := outputs[k]
		if !ok {
			return nil, fmt.Errorf("no output '%s' on resource '%s'", k, st.name)
		}
		return out, nil
	})
}

func (st *lateboundCustomResourceState) CustomResource() *pulumi.CustomResourceState {
	return &st.CustomResourceState
}

func (st *lateboundCustomResourceState) ProviderResource() *pulumi.ProviderResourceState {
	return nil
}

func (*lateboundCustomResourceState) ElementType() reflect.Type {
	return reflect.TypeOf((**lateboundCustomResourceState)(nil)).Elem()
}

func (st *lateboundCustomResourceState) GetRawOutputs() pulumi.Output {
	return pulumi.InternalGetRawOutputs(&st.ResourceState)
}

func (st *lateboundCustomResourceState) GetResourceSchema() *schema.Resource {
	return st.resourceSchema
}

type lateboundProviderResourceState struct {
	pulumi.ProviderResourceState
	name           string
	Outputs        pulumi.MapOutput `pulumi:""`
	resourceSchema *schema.Resource
}

// GetOutputs returns the resource's outputs.
func (st *lateboundProviderResourceState) GetOutputs() pulumi.Output {
	return st.Outputs
}

// GetOutput returns the named output of the resource.
func (st *lateboundProviderResourceState) GetOutput(k string) pulumi.Output {
	return st.Outputs.ApplyT(func(outputs map[string]interface{}) (interface{}, error) {
		out, ok := outputs[k]
		if !ok {
			return nil, fmt.Errorf("no output '%s' on resource '%s'", k, st.name)
		}
		return out, nil
	})
}

func (st *lateboundProviderResourceState) CustomResource() *pulumi.CustomResourceState {
	return &st.CustomResourceState
}

func (st *lateboundProviderResourceState) ProviderResource() *pulumi.ProviderResourceState {
	return &st.ProviderResourceState
}

func (*lateboundProviderResourceState) ElementType() reflect.Type {
	return reflect.TypeOf((**lateboundProviderResourceState)(nil)).Elem()
}

func (st *lateboundProviderResourceState) GetRawOutputs() pulumi.Output {
	return pulumi.InternalGetRawOutputs(&st.ResourceState)
}

func (st *lateboundProviderResourceState) GetResourceSchema() *schema.Resource {
	return st.resourceSchema
}

type poisonMarker struct{}

// GetOutputs returns the resource's outputs.
func (st poisonMarker) GetOutputs() pulumi.Output {
	return nil
}

// GetOutput returns the named output of the resource.
func (st poisonMarker) GetOutput(k string) pulumi.Output {
	return nil
}

func (st poisonMarker) CustomResource() *pulumi.CustomResourceState {
	return nil
}

func (st poisonMarker) ProviderResource() *pulumi.ProviderResourceState {
	return nil
}

func (poisonMarker) ElementType() reflect.Type {
	return reflect.TypeOf((*lateboundResource)(nil)).Elem()
}

func (st poisonMarker) GetRawOutputs() pulumi.Output {
	return nil
}

func (st poisonMarker) GetResourceSchema() *schema.Resource {
	return nil
}

// Check if a value is either a poisonMarker or is a collection that contains a
// poisonMarker.
func isPoisoned(v interface{}) (poisonMarker, bool) {
	switch v := v.(type) {
	case []interface{}:
		for _, e := range v {
			if p, ok := isPoisoned(e); ok {
				return p, true
			}
		}
	case map[string]interface{}:
		for _, e := range v {
			if p, ok := isPoisoned(e); ok {
				return p, true
			}
		}
	case poisonMarker:
		return v, true
	}
	return poisonMarker{}, false
}

func newRunner(t ast.Template, p PackageLoader) *Runner {
	return &Runner{
		t:         t,
		pkgLoader: p,
		config:    make(map[string]interface{}),
		variables: make(map[string]interface{}),
		resources: make(map[string]lateboundResource),
		stackRefs: make(map[string]*pulumi.StackReference),
	}
}

const PulumiVarName = "pulumi"

type Evaluator interface {
	EvalConfig(r *Runner, node configNode) bool
	EvalVariable(r *Runner, node variableNode) bool
	EvalResource(r *Runner, node resourceNode) bool
	EvalOutput(r *Runner, node ast.PropertyMapEntry) bool
	EvalMissing(r *Runner, node missingNode) bool
}

type programEvaluator struct {
	*evalContext
	pulumiCtx   *pulumi.Context
	packageRefs map[tokens.Package]string
}

func (e *programEvaluator) error(expr ast.Expr, summary string) (interface{}, bool) {
	diag := ast.ExprError(expr, summary, "")
	e.addDiag(diag)
	return nil, false
}

func (e *programEvaluator) addDiag(diag *syntax.Diagnostic) {
	defer func() {
		e.sdiags.Extend(diag)
		e.Runner.sdiags.Extend(diag)
	}()

	var buf bytes.Buffer
	w := e.t.NewDiagnosticWriter(&buf, 0, false)
	err := w.WriteDiagnostic(diag.HCL())
	if err != nil {
		err = e.pulumiCtx.Log.Error(fmt.Sprintf("internal error: %v", err), &pulumi.LogArgs{})
	} else {
		s := buf.String()
		// We strip off the appropriate HCL error message, since it will be
		// added back on via the pulumi.Log framework.
		switch diag.Severity {
		case hcl.DiagWarning:
			s = strings.TrimPrefix(s, "Warning: ")
			err = e.pulumiCtx.Log.Warn(s, &pulumi.LogArgs{})
		default:
			s = strings.TrimPrefix(s, "Error: ")
			err = e.pulumiCtx.Log.Error(s, &pulumi.LogArgs{})
		}
	}
	if err != nil {
		os.Stderr.Write([]byte(err.Error()))
	} else {
		diag.Shown = true
	}
}

func (e *programEvaluator) errorf(expr ast.Expr, format string, a ...interface{}) (interface{}, bool) {
	return e.error(expr, fmt.Sprintf(format, a...))
}

func (e programEvaluator) EvalConfig(r *Runner, node configNode) bool {
	ctx := r.newContext(node)
	c, ok := e.registerConfig(node)
	if !ok {
		e.config[node.key().Value] = poisonMarker{}
		msg := fmt.Sprintf("Error registering config [%v]: %v", node.key().Value, ctx.sdiags.Error())
		err := e.pulumiCtx.Log.Error(msg, &pulumi.LogArgs{}) //nolint:errcheck
		if err != nil {
			return false
		}
	} else {
		e.config[node.key().Value] = c
	}
	return true
}

func (e programEvaluator) EvalVariable(r *Runner, node variableNode) bool {
	ctx := r.newContext(node)
	value, ok := e.evaluateExpr(node.Value)
	if !ok {
		e.variables[node.Key.Value] = poisonMarker{}
		msg := fmt.Sprintf("Error registering variable [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := e.pulumiCtx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else {
		e.variables[node.Key.Value] = value
	}
	return true
}

func (e programEvaluator) EvalResource(r *Runner, node resourceNode) bool {
	ctx := r.newContext(node)
	res, ok := e.registerResource(node)
	if !ok {
		e.resources[node.Key.Value] = poisonMarker{}
		msg := fmt.Sprintf("Error registering resource [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := e.pulumiCtx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else {
		e.resources[node.Key.Value] = res
	}
	return true
}

func (e programEvaluator) EvalMissing(r *Runner, node missingNode) bool {
	e.error(node.key(), fmt.Sprintf("resource, variable, or config value %q not found", node.key().Value))
	return false
}

func (e programEvaluator) EvalOutput(r *Runner, node ast.PropertyMapEntry) bool {
	ctx := r.newContext(node)
	out, ok := e.registerOutput(node)
	if !ok {
		msg := fmt.Sprintf("Error registering output [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := e.pulumiCtx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else if _, poisoned := out.(poisonMarker); !poisoned {
		e.pulumiCtx.Export(node.Key.Value, out)
	}
	return true
}

func findPackageRefs(ctx *pulumi.Context, r *Runner) (map[tokens.Package]string, syntax.Diagnostics) {
	// Register package refs for all packages we know upfront
	packageRefs := make(map[tokens.Package]string)
	for key, pkg := range r.packageDescriptors {
		name := pkg.Name
		// If parametrized use the parametrized name
		var parameterization *pulumirpc.Parameterization
		if pkg.Parameterization != nil {
			key = tokens.Package(pkg.Parameterization.Name)

			parameterization = &pulumirpc.Parameterization{
				Name:    pkg.Parameterization.Name,
				Version: pkg.Parameterization.Version.String(),
				Value:   pkg.Parameterization.Value,
			}
		}

		var version string
		if pkg.Version != nil {
			version = pkg.Version.String()
		}

		resp, err := ctx.RegisterPackage(&pulumirpc.RegisterPackageRequest{
			Name:             pkg.Name,
			Version:          version,
			DownloadUrl:      pkg.DownloadURL,
			Parameterization: parameterization,
		})
		if err != nil {
			err = fmt.Errorf("registering package %s: %w", name, err)
			return nil, syntax.Diagnostics{syntax.Error(nil, err.Error(), "")}
		}
		packageRefs[key] = resp.Ref
	}
	return packageRefs, nil
}

func (r *Runner) Evaluate(ctx *pulumi.Context) syntax.Diagnostics {
	eCtx := r.newContext(nil)

	packageRefs, diags := findPackageRefs(ctx, r)
	if diags != nil {
		return diags
	}

	return r.Run(programEvaluator{
		evalContext: eCtx,
		pulumiCtx:   ctx,
		packageRefs: packageRefs,
	})
}

func getConfNodesFromMap(project string, configPropertyMap resource.PropertyMap) []configNode {
	projPrefix := project + ":"
	nodes := make([]configNode, len(configPropertyMap))
	idx := 0
	for k, v := range configPropertyMap {
		n := configNodeProp{
			k: strings.TrimPrefix(string(k), projPrefix),
			v: v,
		}
		nodes[idx] = n
		idx++
	}
	return nodes
}

// setIntermediates is called for convert and runtime evaluation
//
// If force is true, set intermediates even if errors were encountered
// Errors will always be reflected in r.sdiags.
func (r *Runner) setIntermediates(project string, configPropertyMap resource.PropertyMap, force bool) {
	if r.intermediates != nil {
		return
	}

	r.intermediates = []graphNode{}
	confNodes := getConfNodesFromMap(project, configPropertyMap)

	// Topologically sort the intermediates based on implicit and explicit dependencies
	intermediates, rdiags := topologicallySortedResources(r.t, confNodes)
	r.sdiags.Extend(rdiags...)
	if rdiags.HasErrors() && !force {
		return
	}
	if intermediates != nil {
		r.intermediates = intermediates
	}
}

// ensureSetup is called at runtime evaluation
func (r *Runner) ensureSetup(ctx *pulumi.Context) {
	// Our tests need to set intermediates, even though they don't have runtime config
	r.setIntermediates("", nil, false)

	cwd, err := os.Getwd()
	if err != nil {
		r.sdiags.Extend(syntax.Error(nil, err.Error(), ""))
		return
	}

	var project, stack, organization, rootDirectory string
	if ctx != nil {
		project = ctx.Project()
		stack = ctx.Stack()
		organization = ctx.Organization()
		rootDirectory = ctx.RootDirectory()
	}
	r.variables[PulumiVarName] = map[string]interface{}{
		"cwd":           cwd,
		"organization":  organization,
		"project":       project,
		"rootDirectory": rootDirectory,
		"stack":         stack,
	}
	r.cwd = cwd
}

func (r *Runner) Run(e Evaluator) syntax.Diagnostics {
	var ctx *pulumi.Context

	switch eval := e.(type) {
	case programEvaluator:
		ctx = eval.pulumiCtx
	}
	r.ensureSetup(ctx)

	returnDiags := func() syntax.Diagnostics {
		r.sdiags.mutex.Lock()
		defer r.sdiags.mutex.Unlock()
		return r.sdiags.diags
	}
	if r.sdiags.HasErrors() {
		return returnDiags()
	}

	for _, kvp := range r.intermediates {
		switch kvp := kvp.(type) {
		case configNode:
			if _, ok := kvp.(configNodeYaml); ok {
				key := kvp.key().Value
				if _, ok := r.config[key]; ok {
					if ctx != nil {
						err := ctx.Log.Debug(fmt.Sprintf("config already registered %v\n", key), &pulumi.LogArgs{})
						if err != nil {
							return returnDiags()
						}
					}

					continue
				}
			}

			if ctx != nil {
				err := ctx.Log.Debug(fmt.Sprintf("Registering config [%v]", kvp.key().Value), &pulumi.LogArgs{})
				if err != nil {
					return returnDiags()
				}
			}

			if !e.EvalConfig(r, kvp) {
				return returnDiags()
			}
		case variableNode:
			if ctx != nil {
				err := ctx.Log.Debug(fmt.Sprintf("Registering variable [%v]", kvp.Key.Value), &pulumi.LogArgs{})
				if err != nil {
					return returnDiags()
				}
			}
			if !e.EvalVariable(r, kvp) {
				return returnDiags()
			}
		case resourceNode:
			if ctx != nil {
				err := ctx.Log.Debug(fmt.Sprintf("Registering resource [%v]", kvp.Key.Value), &pulumi.LogArgs{})
				if err != nil {
					return returnDiags()
				}
			}
			if !e.EvalResource(r, kvp) {
				return returnDiags()
			}
		case missingNode:
			if !e.EvalMissing(r, kvp) {
				return returnDiags()
			}
		}
	}

	for _, kvp := range r.t.GetOutputs().Entries {
		if !e.EvalOutput(r, kvp) {
			return returnDiags()
		}
	}

	return returnDiags()
}

func (e *programEvaluator) registerConfig(intm configNode) (interface{}, bool) {
	var expectedType ctypes.Type
	var isSecretInConfig, markSecret bool
	var defaultValue interface{}
	var k string
	var intmKey ast.Expr

	switch intm := intm.(type) {
	case configNodeYaml:
		k, intmKey = intm.Key.Value, intm.Key
		c := intm.Value
		if c.Name != nil && c.Name.Value != "" {
			k = c.Name.Value
		}
		// If we implement global type checking, the type of configuration variables
		// can be inferred and this requirement relaxed.
		if c.Type == nil && c.Default == nil {
			return e.errorf(intm.Key, "unable to infer type: either 'default' or 'type' is required")
		}
		if c.Type != nil {
			t, ok := ctypes.Parse(c.Type.Value)
			if !ok {
				return e.errorf(c.Type,
					"unexpected configuration type '%s': valid types are %s",
					c.Type.Value, ctypes.ConfigTypes)
			}

			expectedType = t

		}

		if c.Default != nil {
			var ok bool
			defaultValue, ok = e.evaluateExpr(c.Default)
			if !ok {
				return nil, false
			}
			var err error
			t, err := ctypes.TypeValue(defaultValue)
			if err != nil {
				return e.error(c.Default, err.Error())
			}

			if expectedType == ctypes.Int && t == ctypes.Number && math.Mod(defaultValue.(float64), 1) == 0.0 {
				t = ctypes.Int
				defaultValue = int(defaultValue.(float64))
			}

			// We have both a default value and a explicit type. Make sure they
			// agree.
			if ctypes.IsValidType(expectedType) && t != expectedType {
				return e.errorf(intm.Key,
					"type mismatch: default value of type %s but type %s was specified",
					t, expectedType)
			}

			expectedType = t
		}

		// A value is considered secret if either it is either marked as secret in
		// the config section or the configuration section.
		//
		// We only want to execute a TrySecret* if the value is secret in the config
		// section. It the value is specified as secret only in the configuration
		// section, we call Try* normally, and later wrap the value with
		// `pulumi.ToSecret`.
		isSecretInConfig = e.pulumiCtx.IsConfigSecret(e.pulumiCtx.Project() + ":" + k)

		if isSecretInConfig && c.Secret != nil && !c.Secret.Value {
			return e.error(c.Secret,
				"Cannot mark a configuration value as not secret"+
					" if the associated config value is secret")
		}

		// We only want to mark a value as secret if it is not already secret. If
		// isSecretInConfig is true, we will retrieve a secret value and thus won't need
		// to mark it as secret (since it already will be).
		if (c.Secret != nil && c.Secret.Value) && !isSecretInConfig {
			markSecret = true
		}
	case configNodeProp:
		v := intm.value()
		if intm.v.IsSecret() {
			v = pulumi.ToSecret(intm.v)
		}
		return v, true
	default:
		v := intm.value()
		if e.pulumiCtx.IsConfigSecret(intm.key().GetValue()) {
			v = pulumi.ToSecret(v)
		}
		return v, true
	}

	var v interface{}
	var err error
	switch expectedType {
	case ctypes.String:
		if isSecretInConfig {
			v, err = config.TrySecret(e.pulumiCtx, k)
		} else {
			v, err = config.Try(e.pulumiCtx, k)
		}
	case ctypes.Number:
		if isSecretInConfig {
			v, err = config.TrySecretFloat64(e.pulumiCtx, k)
		} else {
			v, err = config.TryFloat64(e.pulumiCtx, k)
		}
	case ctypes.Int:
		if isSecretInConfig {
			v, err = config.TrySecretInt(e.pulumiCtx, k)
		} else {
			v, err = config.TryInt(e.pulumiCtx, k)
		}
	case ctypes.Boolean:
		if isSecretInConfig {
			v, err = config.TrySecretBool(e.pulumiCtx, k)
		} else {
			v, err = config.TryBool(e.pulumiCtx, k)
		}
	case ctypes.Object:
		var obj map[string]interface{}
		if isSecretInConfig {
			v, err = config.TrySecretObject(e.pulumiCtx, k, &obj)
		} else {
			err = config.TryObject(e.pulumiCtx, k, &obj)
			if err == nil {
				v = obj
			}
		}
	case ctypes.NumberList:
		var arr []float64
		if isSecretInConfig {
			v, err = config.TrySecretObject(e.pulumiCtx, k, &arr)
		} else {
			err = config.TryObject(e.pulumiCtx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	case ctypes.IntList:
		var arr []int
		if isSecretInConfig {
			v, err = config.TrySecretObject(e.pulumiCtx, k, &arr)
		} else {
			err = config.TryObject(e.pulumiCtx, k, &arr)
			if err != nil {
				v = arr
			}
		}
	case ctypes.StringList:
		var arr []string
		if isSecretInConfig {
			v, err = config.TrySecretObject(e.pulumiCtx, k, &arr)
		} else {
			err = config.TryObject(e.pulumiCtx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	case ctypes.BooleanList:
		var arr []bool
		if isSecretInConfig {
			v, err = config.TrySecretObject(e.pulumiCtx, k, &arr)
		} else {
			err = config.TryObject(e.pulumiCtx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	case ctypes.ObjectList:
		var arr []map[string]interface{}
		if isSecretInConfig {
			v, err = config.TrySecretObject(e.pulumiCtx, k, &arr)
		} else {
			err = config.TryObject(e.pulumiCtx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	}

	if errors.Is(err, config.ErrMissingVar) && defaultValue != nil {
		v = defaultValue
	} else if err != nil {
		return e.errorf(intmKey, "%s", err.Error())
	}

	contract.Assertf(v != nil, "let an uninitialized var slip through")

	// The value was marked secret in the configuration section, but in the
	// config section. We need to wrap it in `pulumi.ToSecret`.
	if markSecret {
		v = pulumi.ToSecret(v)
	}

	return v, true
}

func (e *programEvaluator) registerResource(kvp resourceNode) (lateboundResource, bool) {
	return e.registerResourceWithParent(kvp, nil)
}

func (e *programEvaluator) registerResourceWithParent(kvp resourceNode, parent pulumi.Resource) (lateboundResource, bool) {
	k, v := kvp.Key.Value, kvp.Value

	// Read the properties and then evaluate them in case there are expressions contained inside.
	props := make(map[string]interface{})
	overallOk := true

	var opts []pulumi.ResourceOption
	version, err := ParseVersion(v.Options.Version)
	if err != nil {
		e.error(v.Options.Version, fmt.Sprintf("error parsing version of resource %v: %v", k, err))
		return nil, true
	}
	if version != nil {
		opts = append(opts, pulumi.Version(version.String()))
	}

	pluginDownloadURL := ""
	if v.Options.PluginDownloadURL != nil {
		pluginDownloadURL = v.Options.PluginDownloadURL.Value
	}
	pkg, typ, err := ResolveResource(context.TODO(), e.pkgLoader, e.packageDescriptors, v.Type.Value, version,
		pluginDownloadURL)
	if err != nil {
		e.error(v.Type, fmt.Sprintf("error resolving type of resource %v: %v", kvp.Key.Value, err))
		overallOk = false
	}

	readIntoProperties := func(obj ast.PropertyMapDecl) (poisonMarker, bool) {
		for _, kvp := range obj.Entries {
			vv, ok := e.evaluateExpr(kvp.Value)
			if !ok {
				overallOk = false
			}
			if p, ok := vv.(poisonMarker); ok {
				return p, true
			}
			// check if we need to secret-ify the value
			secret, err := pkg.IsResourcePropertySecret(typ, kvp.Key.Value)
			if err != nil {
				e.addWarnDiag(
					kvp.Key.Syntax().Syntax().Range(),
					fmt.Sprintf("error checking if property %v is secret: %v", kvp.Key.Value, err), "")
			}

			if secret {
				vv = pulumi.ToSecret(vv)
			}
			props[kvp.Key.Value] = vv
		}
		return poisonMarker{}, false
	}

	if v.Properties.PropertyMap != nil {
		if p, isPoison := readIntoProperties(*v.Properties.PropertyMap); isPoison {
			return p, isPoison
		}
	} else if v.Properties.Expr != nil {
		// Evaluate the properties
		pm, ok := e.evaluateExpr(v.Properties.Expr)
		if !ok {
			overallOk = false
		}
		if p, ok := pm.(poisonMarker); ok {
			return p, true
		}

		obj, ok := pm.(map[string]interface{})
		if ok {
			// If it's a simple map then just copy the values in it over
			for key, value := range obj {
				// Convert null PropertyValues to nil to avoid passing them to component providers
				if pv, ok := value.(resource.PropertyValue); ok && pv.IsNull() {
					value = nil
				}

				// check if we need to secret-ify the value
				secret, err := pkg.IsResourcePropertySecret(typ, key)
				if err != nil {
					e.addWarnDiag(
						kvp.Key.Syntax().Syntax().Range(),
						fmt.Sprintf("error checking if property %v is secret: %v", kvp.Key.Value, err), "")
				}

				if secret {
					value = pulumi.ToSecret(value)
				}
				props[key] = value
			}
		} else if output, ok := pm.(pulumi.Output); ok {
			// Else it's an output, so we need to ApplyT it to get the values and we don't staticly know what
			// they are so we've got to just set every input the resource says it has.
			hint := pkg.ResourceTypeHint(typ)
			for _, prop := range hint.Resource.InputProperties {
				props[prop.Name] = output.ApplyT(func(v interface{}) (interface{}, error) {
					obj, ok := v.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("expected a map but got %T", v)
					}
					value := obj[prop.Name]

					// check if we need to secret-ify the value
					secret, err := pkg.IsResourcePropertySecret(typ, prop.Name)
					if err != nil {
						e.addWarnDiag(
							kvp.Key.Syntax().Syntax().Range(),
							fmt.Sprintf("error checking if property %v is secret: %v", kvp.Key.Value, err), "")
					}

					if secret {
						value = pulumi.ToSecret(value)
					}

					return value, nil
				})
			}
		} else {
			overallOk = false
			e.addWarnDiag(
				v.Properties.Syntax().Syntax().Range(),
				fmt.Sprintf("properties must be a plain object value was %T", pm), "")
		}
	}

	if v.Options.Parent == nil && parent != nil {
		opts = append(opts, pulumi.Parent(parent))
	}

	if v.Options.Aliases != nil {
		var aliases []pulumi.Alias
		for _, s := range v.Options.Aliases.Elements {
			alias := pulumi.Alias{
				URN: pulumi.URN(s.Value),
			}
			aliases = append(aliases, alias)
		}
		opts = append(opts, pulumi.Aliases(aliases))
	}
	if v.Options.CustomTimeouts != nil {
		var cts pulumi.CustomTimeouts
		if v.Options.CustomTimeouts.Create != nil {
			cts.Create = v.Options.CustomTimeouts.Create.Value
		}
		if v.Options.CustomTimeouts.Update != nil {
			cts.Update = v.Options.CustomTimeouts.Update.Value
		}
		if v.Options.CustomTimeouts.Delete != nil {
			cts.Delete = v.Options.CustomTimeouts.Delete.Value
		}

		opts = append(opts, pulumi.Timeouts(&cts))
	}
	if v.Options.DeleteBeforeReplace != nil {
		opts = append(opts, pulumi.DeleteBeforeReplace(v.Options.DeleteBeforeReplace.Value))
	}
	if v.Options.DependsOn != nil {
		dependOnOpt, ok := e.evaluateResourceListValuedOption(v.Options.DependsOn, "dependsOn")
		if ok {
			var dependsOn []pulumi.Resource
			for _, r := range dependOnOpt {
				if p, ok := r.(poisonMarker); ok {
					return p, true
				}
				dependsOn = append(dependsOn, r.CustomResource())
			}
			opts = append(opts, pulumi.DependsOn(dependsOn))
		} else {
			overallOk = false
		}
	}
	if v.Options.Import != nil {
		opts = append(opts, pulumi.Import(pulumi.ID(v.Options.Import.Value)))
	}
	if v.Options.IgnoreChanges != nil {
		opts = append(opts, pulumi.IgnoreChanges(listStrings(v.Options.IgnoreChanges)))
	}
	if v.Options.Parent != nil {
		parentOpt, ok := e.evaluateResourceValuedOption(v.Options.Parent, "parent")
		if ok {
			if p, ok := parentOpt.(poisonMarker); ok {
				return p, true
			}
			opts = append(opts, pulumi.Parent(parentOpt.CustomResource()))
		} else {
			overallOk = false
		}
	}
	if v.Options.Protect != nil {
		protectValue, ok := e.evaluateExpr(v.Options.Protect)
		if ok {
			if !hasOutputs(protectValue) {
				protect, ok := protectValue.(bool)
				if ok {
					opts = append(opts, pulumi.Protect(protect))
				} else {
					e.error(v.Options.Protect, "protect must be a boolean value")
					overallOk = false
				}
			} else {
				e.error(v.Options.Protect, "protect must be not be an output")
				overallOk = false
			}
		} else {
			e.error(v.Options.Protect, "couldn't evaluate the 'protect' resource option")
			overallOk = false
		}
	}

	if v.Options.Provider != nil {
		providerOpt, ok := e.evaluateResourceValuedOption(v.Options.Provider, "provider")
		if ok {
			if p, ok := providerOpt.(poisonMarker); ok {
				return p, true
			}
			provider := providerOpt.ProviderResource()

			if provider == nil {
				e.error(v.Options.Provider, fmt.Sprintf("resource passed as Provider was not a provider resource '%s'", providerOpt))
			} else {
				opts = append(opts, pulumi.Provider(provider))
			}
		} else {
			overallOk = false
		}
	}
	if v.Options.Providers != nil {
		dependOnOpt, ok := e.evaluateResourceListValuedOption(v.Options.Providers, "providers")
		if ok {
			var providers []pulumi.ProviderResource
			for _, r := range dependOnOpt {
				if p, ok := r.(poisonMarker); ok {
					return p, true
				}
				provider := r.ProviderResource()
				if provider == nil {
					e.error(v.Options.Provider, fmt.Sprintf("resource passed as provider was not a provider resource '%s'", r))
				} else {
					providers = append(providers, provider)
				}
			}
			opts = append(opts, pulumi.Providers(providers...))
		} else {
			overallOk = false
		}
	}

	if v.Options.PluginDownloadURL != nil {
		opts = append(opts, pulumi.PluginDownloadURL(v.Options.PluginDownloadURL.Value))
	}
	if v.Options.ReplaceOnChanges != nil {
		opts = append(opts, pulumi.ReplaceOnChanges(listStrings(v.Options.ReplaceOnChanges)))
	}
	if b := v.Options.RetainOnDelete; b != nil {
		opts = append(opts, pulumi.RetainOnDelete(b.Value))
	}
	if v.Options.ReplaceWith != nil {
		replaceWithOpt, ok := e.evaluateResourceListValuedOption(v.Options.ReplaceWith, "replaceWith")
		if ok {
			var replaceWith []pulumi.Resource
			for _, r := range replaceWithOpt {
				if p, ok := r.(poisonMarker); ok {
					return p, true
				}
				replaceWith = append(replaceWith, r.CustomResource())
			}
			opts = append(opts, pulumi.ReplaceWith(replaceWith))
		} else {
			overallOk = false
		}
	}
	if v.Options.DeletedWith != nil {
		deletedWithOpt, ok := e.evaluateResourceValuedOption(v.Options.DeletedWith, "deletedWith")
		if ok {
			if p, ok := deletedWithOpt.(poisonMarker); ok {
				return p, true
			}
			opts = append(opts, pulumi.DeletedWith(deletedWithOpt.CustomResource()))
		} else {
			overallOk = false
		}
	}
	if v.Options.HideDiffs != nil {
		opts = append(opts, pulumi.HideDiffs(listStrings(v.Options.HideDiffs)))
	}

	// Create either a latebound custom resource or latebound provider resource depending on
	// whether the type token indicates a special provider type.
	resourceName := k
	if v.Name != nil && v.Name.Value != "" {
		resourceName = v.Name.Value
	}

	var state lateboundResource
	var res pulumi.Resource
	var resourceSchema *schema.Resource
	if resType := pkg.ResourceTypeHint(typ); resType != nil {
		resourceSchema = resType.Resource
	}
	isProvider := false
	if strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
		r := lateboundProviderResourceState{name: resourceName, resourceSchema: resourceSchema}
		state = &r
		res = &r
		isProvider = true
	} else {
		r := lateboundCustomResourceState{name: resourceName, resourceSchema: resourceSchema}
		state = &r
		res = &r
	}
	if v.Options.AdditionalSecretOutputs != nil {
		opts = append(opts, pulumi.AdditionalSecretOutputs(listStrings(v.Options.AdditionalSecretOutputs)))
	}
	for _, prop := range resourceSchema.Properties {
		if prop.Secret {
			opts = append(opts, pulumi.AdditionalSecretOutputs([]string{prop.Name}))
		}
	}
	for _, alias := range resourceSchema.Aliases {
		if alias.Type != "" {
			opts = append(opts, pulumi.Aliases([]pulumi.Alias{
				{Type: pulumi.String(alias.Type)},
			}))
		}
	}

	if !overallOk || e.sdiags.HasErrors() {
		return nil, false
	}

	isComponent := false
	if !isProvider {
		result, err := pkg.IsComponent(typ)
		if err != nil {
			e.error(v.Type, "unable to resolve type")
			return nil, false
		}
		isComponent = result
	}

	constants := pkg.ResourceConstants(typ)
	for k, v := range constants {
		props[k] = v
	}

	// For a StackReference we always use the name property as ID. We patch up
	// the resource declaration's ID with this name.
	isStackReference := v.Type.Value == "pulumi:pulumi:StackReference"
	if isStackReference {
		nameProp, ok := props["name"]
		if !ok {
			nameProp = k
			props["name"] = k
		}
		name, ok := nameProp.(string)
		if !ok {
			e.errorf(kvp.Key, "'name' property must be a string, instead got type %T", name)
			return nil, false
		}
		v.Get.Id = ast.String(name)
	}

	isRead := v.Get.Id != nil
	if isRead && !isStackReference { // StackReferences have a required name property
		contract.Assertf(len(props) == 0, "Failed to check that Properties cannot be specified with Get.State")
		p, isPoison := readIntoProperties(v.Get.State)
		if isPoison {
			return p, true
		}
	}

	// Now register the resulting resource with the engine.
	if isComponent {
		typ := tokens.Type(typ)
		packageRef := e.packageRefs[typ.Package()]
		err = e.pulumiCtx.RegisterPackageRemoteComponentResource(string(typ), resourceName, untypedArgs(props), res, packageRef, opts...)
	} else if isRead {
		s, ok := e.evaluateExpr(v.Get.Id)
		if !ok {
			e.error(v.Get.Id, "unable to evaluate get.id")
			return nil, false
		}

		convertID := func(a any) (pulumi.ID, error) {
			s, ok := a.(string)
			if !ok {
				err := typeCheckerError{
					expected: "string",
					found:    fmt.Sprintf("%T", a),
					location: v.Get.Id,
				}
				e.addDiag(err.Diag())
				return "", err
			}
			return pulumi.ID(s), nil
		}
		var id pulumi.IDInput
		switch s := s.(type) {
		case poisonMarker:
			return s, true
		case string:
			id = pulumi.ID(s)
		case pulumi.StringOutput:
			id = s.ApplyT(convertID).(pulumi.IDOutput)
		case pulumi.AnyOutput:
			id = s.ApplyT(convertID).(pulumi.IDOutput)
		default:
			err := typeCheckerError{
				expected: "string",
				found:    fmt.Sprintf("%T", s),
				location: v.Get.Id,
			}

			e.addDiag(err.Diag())
			return nil, false
		}
		err = e.pulumiCtx.ReadResource(
			string(typ),
			resourceName,
			id,
			untypedArgs(props),
			res.(pulumi.CustomResource),
			opts...)
	} else {
		typ := tokens.Type(typ)

		pkg := typ.Package()
		if typ.Module() == "pulumi:providers" {
			// This is a provider resource, so the package is actually the name in the token, not the package.
			pkg = tokens.Package(typ.Name())
		}
		packageRef := e.packageRefs[pkg]
		err = e.pulumiCtx.RegisterPackageResource(string(typ), resourceName, untypedArgs(props), res, packageRef, opts...)
	}
	if err != nil {
		e.error(kvp.Key, err.Error())
		return nil, false
	}

	return state, true
}

func (e *programEvaluator) evaluateResourceListValuedOption(optionExpr ast.Expr, key string) ([]lateboundResource, bool) {
	value, ok := e.evaluateExpr(optionExpr)
	if !ok {
		return nil, false
	}
	if hasOutputs(value) {
		e.error(optionExpr, fmt.Sprintf("resource option %v value must be a list of resource, not an output", key))
		return nil, false
	}
	dependencies, ok := value.([]interface{})
	if !ok {
		e.error(optionExpr, fmt.Sprintf("resource option %v value must be a list of resources", key))
		return nil, false
	}
	var resources []lateboundResource
	for _, dep := range dependencies {
		res, err := asResource(dep)
		if err != nil {
			e.error(optionExpr, err.Error())
			continue
		}
		resources = append(resources, res)
	}
	return resources, true
}

func (e *programEvaluator) evaluateResourceValuedOption(optionExpr ast.Expr, key string) (lateboundResource, bool) {
	value, ok := e.evaluateExpr(optionExpr)
	if !ok {
		return nil, false
	}
	if hasOutputs(value) {
		e.error(optionExpr, "resource cannot be an output")
		return nil, false
	}
	res, err := asResource(value)
	if err != nil {
		e.error(optionExpr, err.Error())
		return nil, false
	}
	return res, true
}

func asResource(value interface{}) (lateboundResource, error) {
	switch d := value.(type) {
	case lateboundResource:
		return d, nil
	default:
		return nil, fmt.Errorf("expected resource, got %v", reflect.TypeOf(value))
	}
}

func (e *programEvaluator) registerOutput(kvp ast.PropertyMapEntry) (pulumi.Input, bool) {
	out, ok := e.evaluateExpr(kvp.Value)
	if !ok {
		return nil, false
	}

	switch res := out.(type) {
	case poisonMarker:
		return res, true
	case *lateboundCustomResourceState:
		return res, true
	case *lateboundProviderResourceState:
		return res, true
	default:
		return pulumi.Any(out), true
	}
}

// evaluateExpr evaluates an expression tree. The result must be one of the following types:
//
// - nil
// - string
// - bool
// - float64
// - []interface{}
// - map[string]interface{}
// - pulumi.Output, where the element type is one of the above
func (e *programEvaluator) evaluateExpr(x ast.Expr) (interface{}, bool) {
	switch x := x.(type) {
	case *ast.NullExpr:
		return nil, true
	case *ast.BooleanExpr:
		return x.Value, true
	case *ast.NumberExpr:
		return x.Value, true
	case *ast.StringExpr:
		return x.Value, true
	case *ast.ListExpr:
		return e.evaluateList(x)
	case *ast.ObjectExpr:
		var entries []ast.ObjectProperty
		if x != nil {
			entries = x.Entries
		}
		return e.evaluateObject(x, entries)
	case *ast.InterpolateExpr:
		return e.evaluateInterpolate(x)
	case *ast.SymbolExpr:
		return e.evaluatePropertyAccess(x, x.Property)
	case *ast.InvokeExpr:
		return e.evaluateBuiltinInvoke(x)
	case *ast.JoinExpr:
		return e.evaluateBuiltinJoin(x)
	case *ast.SplitExpr:
		return e.evaluateBuiltinSplit(x)
	case *ast.ToJSONExpr:
		return e.evaluateBuiltinToJSON(x)
	case *ast.SelectExpr:
		return e.evaluateBuiltinSelect(x)
	case *ast.ToBase64Expr:
		return e.evaluateBuiltinToBase64(x)
	case *ast.FromBase64Expr:
		return e.evaluateBuiltinFromBase64(x)
	case *ast.FileAssetExpr:
		return e.evaluateInterpolatedBuiltinAssetArchive(x, x.Source)
	case *ast.StringAssetExpr:
		return e.evaluateInterpolatedBuiltinAssetArchive(x, x.Source)
	case *ast.RemoteAssetExpr:
		return e.evaluateInterpolatedBuiltinAssetArchive(x, x.Source)
	case *ast.FileArchiveExpr:
		return e.evaluateInterpolatedBuiltinAssetArchive(x, x.Source)
	case *ast.RemoteArchiveExpr:
		return e.evaluateInterpolatedBuiltinAssetArchive(x, x.Source)
	case *ast.AssetArchiveExpr:
		return e.evaluateBuiltinAssetArchive(x)
	case *ast.StackReferenceExpr:
		e.addWarnDiag(x.Syntax().Syntax().Range(),
			"'fn::stackReference' is deprecated",
			"Please use `pulumi:pulumi:StackReference`; see"+
				"https://www.pulumi.com/docs/intro/concepts/stack/#stackreferences")
		return e.evaluateBuiltinStackReference(x)
	case *ast.SecretExpr:
		return e.evaluateBuiltinSecret(x)
	case *ast.ReadFileExpr:
		return e.evaluateBuiltinReadFile(x)
	default:
		panic(fmt.Sprintf("fatal: invalid expr type %v", reflect.TypeOf(x)))
	}
}

func (e *programEvaluator) evaluateList(x *ast.ListExpr) (interface{}, bool) {
	xs := make([]interface{}, len(x.Elements))
	for i, elem := range x.Elements {
		ev, ok := e.evaluateExpr(elem)
		if !ok {
			return nil, false
		}
		if p, ok := ev.(poisonMarker); ok {
			return p, true
		}
		xs[i] = ev
	}
	return xs, true
}

func (e *programEvaluator) evaluateObject(x *ast.ObjectExpr, entries []ast.ObjectProperty) (interface{}, bool) {
	if len(entries) == 0 {
		return map[string]interface{}{}, true
	}

	allOk := true
	var keys []interface{}
	var keyExprs []ast.Expr
	var values []interface{}
	for _, op := range entries {
		k, ok := e.evaluateExpr(op.Key)
		if !ok {
			allOk = false
		}
		keys = append(keys, k)
		keyExprs = append(keyExprs, op.Key)

		v, ok := e.evaluateExpr(op.Value)
		if !ok {
			allOk = false
		}
		values = append(values, v)
	}

	if !allOk {
		return nil, false
	}

	evalObjectF := e.lift(func(args ...interface{}) (interface{}, bool) {
		returnMap := map[string]interface{}{}
		allOk := true
		for i, arg := range args {
			if k, ok := arg.(string); ok {
				returnMap[k] = values[i]
			} else {
				e.error(keyExprs[i], fmt.Sprintf("object key must evaluate to a string, not %v", typeString(k)))
				allOk = false
			}
		}

		if !allOk {
			return nil, false
		}

		return returnMap, true
	})

	return evalObjectF(keys...)
}

func (e *programEvaluator) evaluateInterpolate(x *ast.InterpolateExpr) (interface{}, bool) {
	return e.evaluateInterpolations(x, &strings.Builder{}, x.Parts)
}

func (e *programEvaluator) evaluateInterpolations(x *ast.InterpolateExpr, b *strings.Builder, parts []ast.Interpolation) (interface{}, bool) {
	for ; len(parts) > 0; parts = parts[1:] {
		i := parts[0]
		b.WriteString(i.Text)

		if i.Value != nil {
			p, ok := e.evaluatePropertyAccess(x, i.Value)
			if !ok {
				return nil, false
			}
			if p, ok := p.(poisonMarker); ok {
				return p, true
			}

			if o, ok := p.(pulumi.Output); ok {
				return o.ApplyT(func(v interface{}) (interface{}, error) {
					fmt.Fprintf(b, "%v", v)
					v, ok := e.evaluateInterpolations(x, b, parts[1:])
					if !ok {
						return nil, fmt.Errorf("runtime error")
					}
					return v, nil
				}), true
			}

			fmt.Fprintf(b, "%v", p)
		}
	}
	return b.String(), true
}

func unknownOutput() pulumi.Output {
	return pulumi.UnsafeUnknownOutput(nil)
}

// evaluatePropertyAccess evaluates interpolation expressions, `${foo.bar[baz]}`. The first item in
// the property access list is the head, and must be an identifier for a resource, config, or
// variable. The tail of property accessors are either: `.foo` string literal property names or
// `[42]` numeric literal property subscripts.
func (e *programEvaluator) evaluatePropertyAccess(expr ast.Expr, access *ast.PropertyAccess) (interface{}, bool) {
	resourceName := access.RootName()
	var receiver interface{}
	if res, ok := e.resources[resourceName]; ok {
		receiver = res
	} else if p, ok := e.config[resourceName]; ok {
		receiver = p
	} else if v, ok := e.variables[resourceName]; ok {
		receiver = v
	} else if p, ok := e.config[stripConfigNamespace(e.pulumiCtx.Project(), resourceName)]; ok {
		receiver = p
	} else {
		return e.error(expr, fmt.Sprintf("resource or variable named %q could not be found", resourceName))
	}

	return e.evaluatePropertyAccessTail(expr, receiver, access.Accessors[1:])
}

func (e *programEvaluator) evaluatePropertyAccessTail(expr ast.Expr, receiver interface{}, accessors []ast.PropertyAccessor) (interface{}, bool) {
	var evaluateAccessF func(args ...interface{}) (interface{}, bool)
	evaluateAccessF = e.lift(func(args ...interface{}) (interface{}, bool) {
		receiver := args[0]
		accessors := args[1].([]ast.PropertyAccessor)
	Loop:
		for {
			switch x := receiver.(type) {
			case pulumi.Output:
				// If the receiver is an output, we need to apply it to get the value.
				return x.ApplyT(func(v interface{}) (interface{}, error) {
					result, ok := evaluateAccessF(v, accessors)
					if !ok {
						return nil, fmt.Errorf("runtime error")
					}
					return result, nil
				}), true
			case lateboundResource:
				// Peak ahead at the next accessor to implement .urn and .id:
				if len(accessors) >= 1 {
					sub, ok := accessors[0].(*ast.PropertyName)
					if ok && sub.Name == "id" {
						return x.CustomResource().ID().ToStringOutput(), true
					} else if ok && sub.Name == "urn" {
						return x.CustomResource().URN().ToStringOutput(), true
					}

					outputs := x.GetRawOutputs()

					// If we're in a preview, mark missing outputs in the schema as unknown.
					// unknownOutput values will break in an actual deployment.
					if outputs != nil && e.pulumiCtx.DryRun() {
						outputs = outputs.ApplyT(
							func(rawOutputs interface{}) (interface{}, error) {
								outputs, ok := rawOutputs.(resource.PropertyMap)
								if !ok {
									return rawOutputs, nil
								}
								resourceSchema := x.GetResourceSchema()
								if resourceSchema == nil {
									return outputs, nil
								}
								newOutputs := outputs.Copy()
								for _, v := range resourceSchema.Properties {
									if _, ok := newOutputs[resource.PropertyKey(v.Name)]; !ok {
										newOutputs[resource.PropertyKey(v.Name)] = resource.PropertyValue{
											V: unknownOutput(),
										}
									}
								}
								return newOutputs, nil
							})
					}
					return evaluateAccessF(outputs, accessors)
				}
				return x, true
			case resource.PropertyMap:
				if len(accessors) == 0 {
					if x.ContainsUnknowns() {
						return unknownOutput(), true
					}
					receiver = x.Mappable()
					break Loop
				}
				var k string
				switch a := accessors[0].(type) {
				case *ast.PropertyName:
					k = a.Name
				case *ast.PropertySubscript:
					s, ok := a.Index.(string)
					if !ok {
						return e.error(expr, "cannot access an object property using an integer index")
					}
					k = s
				}
				prop, ok := x[resource.PropertyKey(k)]
				if x.ContainsUnknowns() && !ok {
					return unknownOutput(), true
				} else if !ok || prop.IsNull() {
					receiver = nil
				} else {
					// Not-known-to-be-unknown output/computed properties inside maps
					// containing unknowns should be treated as unknown during previews to
					// ensure that we don't end up using old values.
					if e.pulumiCtx.DryRun() && x.ContainsUnknowns() && !prop.ContainsUnknowns() {
						if prop.IsOutput() || prop.IsComputed() {
							return unknownOutput(), true
						}
					}

					receiver = prop
				}
				accessors = accessors[1:]
			case resource.PropertyValue:
				switch {
				case x.IsNull():
					return nil, true
				case x.IsComputed():
					return unknownOutput(), true
				case x.IsOutput():
					if !x.OutputValue().Known {
						return unknownOutput(), true
					}
					receiver = x.OutputValue().Element
				case x.IsSecret():
					return evaluateAccessF(pulumi.ToSecret(x.SecretValue().Element), accessors)
				case x.IsArray():
					receiver = x.ArrayValue()
				case x.IsObject():
					receiver = x.ObjectValue()
				case x.IsAsset():
					receiver = x.AssetValue()
				case x.IsArchive():
					receiver = x.ArchiveValue()
				case x.IsResourceReference():
					ref := x.ResourceReferenceValue()
					var state lateboundResource
					var res pulumi.Resource
					if strings.HasPrefix(string(ref.URN.Type()), "pulumi:providers:") {
						r := lateboundProviderResourceState{name: ""}
						state = &r
						res = &r
					} else {
						r := lateboundCustomResourceState{name: ""}
						state = &r
						res = &r
					}
					// Use the `getResource` invoke to get and deserialize the resource from state:
					err := e.pulumiCtx.RegisterResource("_", "_", nil, res, pulumi.URN_(string(ref.URN)))
					if err != nil {
						e.error(expr, fmt.Sprintf("Failed to get resource %q: %v", ref.URN, err))
						return nil, false
					}
					return evaluateAccessF(state, accessors)
				default:
					receiver = x.V
				}
			case []resource.PropertyValue:
				if len(accessors) == 0 {
					if resource.NewArrayProperty(x).ContainsUnknowns() {
						return unknownOutput(), true
					}
					receiver = resource.NewArrayProperty(x).Mappable()
					break Loop
				}
				sub, ok := accessors[0].(*ast.PropertySubscript)
				if !ok {
					return e.error(expr, "cannot access a list element using a property name")
				}
				index, ok := sub.Index.(int)
				if !ok {
					return e.error(expr, "cannot access a list element using a property name")
				}
				if index < 0 || index >= len(x) {
					return e.error(expr, fmt.Sprintf("list index %v out-of-bounds for list of length %v", index, len(x)))
				}
				receiver = x[index]
				accessors = accessors[1:]
			case []interface{}, []string, []int, []float64, []bool:
				if len(accessors) == 0 {
					break Loop
				}
				sub, ok := accessors[0].(*ast.PropertySubscript)
				if !ok {
					return e.error(expr, "cannot access a list element using a property name")
				}
				index, ok := sub.Index.(int)
				if !ok {
					return e.error(expr, "cannot access a list element using a property name")
				}
				reflx := reflect.ValueOf(x)
				length := reflx.Len()
				if index < 0 || index >= length {
					return e.error(expr, fmt.Sprintf("list index %v out-of-bounds for list of length %v", index, length))
				}
				receiver = reflect.Indirect(reflx).Index(index).Interface()
				accessors = accessors[1:]
			case map[string]interface{}:
				if len(accessors) == 0 {
					break Loop
				}
				var k string
				switch a := accessors[0].(type) {
				case *ast.PropertyName:
					k = a.Name
				case *ast.PropertySubscript:
					s, ok := a.Index.(string)
					if !ok {
						return e.error(expr, "cannot access an object property using an integer index")
					}
					k = s
				}
				receiver = x[k]
				accessors = accessors[1:]
			default:
				if len(accessors) == 0 {
					break Loop
				}
				return e.error(expr, fmt.Sprintf("receiver must be a list or object, not %v", typeString(receiver)))
			}
		}
		return receiver, true
	})

	return evaluateAccessF(receiver, accessors)
}

// evaluateBuiltinInvoke evaluates the "Invoke" builtin, which enables templates to invoke arbitrary
// data source functions, to fetch information like the current availability zone, lookup AMIs, etc.
func (e *programEvaluator) evaluateBuiltinInvoke(t *ast.InvokeExpr) (interface{}, bool) {
	args, ok := e.evaluateExpr(t.CallArgs)
	if !ok {
		return nil, false
	}

	var opts []pulumi.InvokeOption

	if t.CallOpts.Version != nil {
		opts = append(opts, pulumi.Version(t.CallOpts.Version.Value))
	}
	if t.CallOpts.PluginDownloadURL != nil {
		opts = append(opts, pulumi.PluginDownloadURL(t.CallOpts.PluginDownloadURL.Value))
	}
	if t.CallOpts.Parent != nil {
		parentOpt, ok := e.evaluateResourceValuedOption(t.CallOpts.Parent, "parent")
		if ok {
			if p, ok := parentOpt.(poisonMarker); ok {
				return p, true
			}
			opts = append(opts, pulumi.Parent(parentOpt.CustomResource()))
		} else {
			e.error(t.Return, fmt.Sprintf("Unable to evaluate options Parent field: %+v", t.CallOpts.Parent))
		}
	}
	if t.CallOpts.Provider != nil {
		providerOpt, ok := e.evaluateResourceValuedOption(t.CallOpts.Provider, "provider")
		if ok {
			if p, ok := providerOpt.(poisonMarker); ok {
				return p, true
			}
			provider := providerOpt.ProviderResource()
			if provider == nil {
				e.error(t.CallOpts.Provider, fmt.Sprintf("resource passed as Provider was not a provider resource '%s'", providerOpt))
			} else {
				opts = append(opts, pulumi.Provider(provider))
			}
		} else {
			e.error(t.Return, fmt.Sprintf("Unable to evaluate options Provider field: %+v", t.CallOpts.Provider))
		}
	}

	dependsOn := []pulumi.Resource{}
	if t.CallOpts.DependsOn != nil {
		dependsOnOpt, ok := e.evaluateResourceListValuedOption(t.CallOpts.DependsOn, "dependsOn")
		if ok {
			for _, r := range dependsOnOpt {
				if p, ok := r.(poisonMarker); ok {
					return p, true
				}
				dependsOn = append(dependsOn, r.CustomResource())
			}
			opts = append(opts, pulumi.DependsOn(dependsOn))
		} else {
			e.error(t.Return, fmt.Sprintf("Unable to evaluate options DependsOn field: %+v", t.CallOpts.DependsOn))
		}
	}
	performInvoke := e.lift(func(args ...interface{}) (interface{}, bool) {
		// At this point, we've got a function to invoke and some parameters! Invoke away.
		result := map[string]interface{}{}
		version, err := ParseVersion(t.CallOpts.Version)
		if err != nil {
			e.error(t.CallOpts.Version, fmt.Sprintf("unable to parse function provider version: %v", err))
			return nil, true
		}
		pluginDownloadURL := ""
		if t.CallOpts.PluginDownloadURL != nil {
			pluginDownloadURL = t.CallOpts.PluginDownloadURL.Value
		}
		pkg, functionName, err := ResolveFunction(e.pulumiCtx.Context(), e.pkgLoader, e.packageDescriptors, t.Token.Value, version, pluginDownloadURL)
		if err != nil {
			return e.error(t, err.Error())
		}
		hint := pkg.FunctionTypeHint(functionName)

		typ := tokens.Type(functionName)
		packageRef := e.packageRefs[typ.Package()]
		secret, err := e.pulumiCtx.InvokePackageRaw(string(functionName), args[0], &result, packageRef, opts...)
		if err != nil {
			return e.error(t, err.Error())
		}

		// If this is a scalar return extract the single field from the result map.

		singleReturnType := hint.ReturnType
		if _, ok := hint.ReturnType.(*schema.ObjectType); ok || hint.Outputs != nil {
			singleReturnType = nil
		}

		var retv any
		if singleReturnType != nil {
			if len(result) != 1 {
				e.addWarnDiag(
					t.Syntax().Syntax().Range(),
					fmt.Sprintf("fn::invoke of %s expected a single return value, got: %+v", t.Token.Value, result), "")
			}

			if t.Return.GetValue() != "" {
				e.addDiag(ast.ExprError(t.Return,
					fmt.Sprintf("Unable to evaluate result[%v]", t.Return.Value),
					fmt.Sprintf("fn::invoke of %s has a non-object return value; cannot specify property '%s'", t.Token.Value, t.Return.Value),
				))
				return nil, false
			}

			for _, v := range result {
				retv = v
			}
		} else {
			if t.Return.GetValue() == "" {
				retv = result
			} else {
				var ok bool
				retv, ok = result[t.Return.Value]
				if !ok {
					e.addDiag(ast.ExprError(t.Return,
						fmt.Sprintf("Unable to evaluate result[%v], result is: %+v", t.Return.Value, t.Return),
						fmt.Sprintf("fn::invoke of %s did not contain a property '%s' in the returned value", t.Token.Value, t.Return.Value),
					))
					return nil, false
				}
			}
		}

		output := pulumi.OutputWithDependencies(e.pulumiCtx.Context(), pulumi.Any(retv), dependsOn...)
		if secret {
			return pulumi.ToSecret(output), true
		}
		return output, true
	})
	return performInvoke(args)
}

func (e *programEvaluator) evaluateBuiltinJoin(v *ast.JoinExpr) (interface{}, bool) {
	overallOk := true

	delim, ok := e.evaluateExpr(v.Delimiter)
	overallOk = overallOk && ok

	items, ok := e.evaluateExpr(v.Values)
	overallOk = overallOk && ok

	if !overallOk {
		return nil, false
	}

	join := e.lift(func(args ...interface{}) (interface{}, bool) {
		overallOk := true

		delim := args[0]
		if delim == nil {
			delim = ""
		}
		delimStr, ok := delim.(string)
		overallOk = overallOk && ok
		if !ok {
			e.error(v.Delimiter, fmt.Sprintf("delimiter must be a string, not %v", typeString(args[0])))
		}

		parts, ok := args[1].([]interface{})
		overallOk = overallOk && ok
		if !ok {
			e.error(v.Values, fmt.Sprintf("the second argument to fn::join must be a list, found %v", typeString(args[1])))
		}

		if !overallOk {
			return nil, false
		}

		strs := make([]string, len(parts))
		for i, p := range parts {
			str, ok := p.(string)
			if !ok {
				e.error(v.Values, fmt.Sprintf("the second argument to fn::join must be a list of strings, found %v at index %v", typeString(p), i))
				overallOk = false
			} else {
				strs[i] = str
			}
		}

		if !overallOk {
			return "", false
		}

		return strings.Join(strs, delimStr), true
	})
	return join(delim, items)
}

func (e *programEvaluator) evaluateBuiltinSplit(v *ast.SplitExpr) (interface{}, bool) {
	delimiter, delimOk := e.evaluateExpr(v.Delimiter)
	source, sourceOk := e.evaluateExpr(v.Source)
	if !delimOk || !sourceOk {
		return nil, false
	}

	split := e.lift(func(args ...interface{}) (interface{}, bool) {
		d, delimOk := args[0].(string)
		if !delimOk {
			e.error(v.Delimiter, fmt.Sprintf("Must be a string, not %v", typeString(d)))
		}
		s, sourceOk := args[1].(string)
		if !sourceOk {
			e.error(v.Source, fmt.Sprintf("Must be a string, not %v", typeString(s)))
		}
		if !delimOk || !sourceOk {
			return nil, false
		}
		return strings.Split(s, d), true
	})
	return split(delimiter, source)
}

func (e *programEvaluator) evaluateBuiltinToJSON(v *ast.ToJSONExpr) (interface{}, bool) {
	value, ok := e.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}

	toJSON := e.lift(func(args ...interface{}) (interface{}, bool) {
		b, err := json.Marshal(args[0])
		if err != nil {
			e.error(v, fmt.Sprintf("failed to encode JSON: %v", err))
			return "", false
		}
		return string(b), true
	})
	return toJSON(value)
}

func (e *programEvaluator) evaluateBuiltinSelect(v *ast.SelectExpr) (interface{}, bool) {
	index, ok := e.evaluateExpr(v.Index)
	if !ok {
		return nil, false
	}
	values, ok := e.evaluateExpr(v.Values)
	if !ok {
		return nil, false
	}

	selectFn := e.lift(func(args ...interface{}) (interface{}, bool) {
		indexArg := args[0]
		elemsArg := args[1]

		index, ok := indexArg.(float64)
		if !ok {
			return e.error(v.Index, fmt.Sprintf("index must be a number, not %v", typeString(indexArg)))
		}
		if float64(int(index)) != index || int(index) < 0 {
			// Cannot be a valid index, so we error
			f := strconv.FormatFloat(index, 'f', -1, 64) // Manual formatting is so -3 does not get formatted as -3.0
			return e.error(v.Index, fmt.Sprintf("index must be a positive integral, not %s", f))
		}
		intIndex := int(index)

		return e.evaluatePropertyAccessTail(v.Values, elemsArg, []ast.PropertyAccessor{&ast.PropertySubscript{Index: intIndex}})
	})
	return selectFn(index, values)
}

func (e *programEvaluator) evaluateBuiltinFromBase64(v *ast.FromBase64Expr) (interface{}, bool) {
	str, ok := e.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}
	fromBase64 := e.lift(func(args ...interface{}) (interface{}, bool) {
		s, ok := args[0].(string)
		if !ok {
			return e.error(v.Value, fmt.Sprintf("expected argument to fn::fromBase64 to be a string, got %v", typeString(args[0])))
		}
		b, err := b64.StdEncoding.DecodeString(s)
		if err != nil {
			return e.error(v.Value, fmt.Sprintf("fn::fromBase64 unable to decode %v, error: %v", args[0], err))
		}
		decoded := string(b)
		if !utf8.ValidString(decoded) {
			return e.error(v.Value, "fn::fromBase64 output is not a valid UTF-8 string")
		}
		return decoded, true
	})
	return fromBase64(str)
}

func (e *programEvaluator) evaluateBuiltinToBase64(v *ast.ToBase64Expr) (interface{}, bool) {
	str, ok := e.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}
	toBase64 := e.lift(func(args ...interface{}) (interface{}, bool) {
		s, ok := args[0].(string)
		if !ok {
			return e.error(v.Value, fmt.Sprintf("expected argument to fn::toBase64 to be a string, got %v", typeString(args[0])))
		}
		return b64.StdEncoding.EncodeToString([]byte(s)), true
	})
	return toBase64(str)
}

func (e *programEvaluator) evaluateBuiltinAssetArchive(v *ast.AssetArchiveExpr) (interface{}, bool) {
	m := map[string]interface{}{}
	keys := make([]string, len(v.AssetOrArchives))
	i := 0
	for k := range v.AssetOrArchives {
		keys[i] = k
		i++
	}
	sort.Strings(keys)

	overallOk := true

	for _, k := range keys {
		v := v.AssetOrArchives[k]
		assetOrArchive, ok := e.evaluateExpr(v)
		if !ok {
			overallOk = false
		} else {
			m[k] = assetOrArchive
		}
	}

	if !overallOk {
		return nil, false
	}

	return pulumi.NewAssetArchive(m), true
}

func (e *programEvaluator) evaluateBuiltinStackReference(v *ast.StackReferenceExpr) (interface{}, bool) {
	stackRef, ok := e.stackRefs[v.StackName.Value]
	if !ok {
		var err error
		stackRef, err = pulumi.NewStackReference(e.pulumiCtx, v.StackName.Value, &pulumi.StackReferenceArgs{})
		if err != nil {
			return e.error(v.StackName, err.Error())
		}
		e.stackRefs[v.StackName.Value] = stackRef
	}

	property, ok := e.evaluateExpr(v.PropertyName)
	if !ok {
		return nil, false
	}

	propertyStringOutput := pulumi.ToOutput(property).ApplyT(func(n interface{}) (string, error) {
		s, ok := n.(string)
		if !ok {
			e.error(v.PropertyName,
				fmt.Sprintf("expected property name argument to fn::stackReference to be a string, got %v", typeString(n)),
			)
		}
		return s, nil
	}).(pulumi.StringOutput)

	return stackRef.GetOutput(propertyStringOutput), true
}

func (e *programEvaluator) evaluateBuiltinSecret(s *ast.SecretExpr) (interface{}, bool) {
	expr, ok := e.evaluateExpr(s.Value)
	if !ok {
		return nil, false
	}
	return pulumi.ToSecret(expr), true
}

func (e *programEvaluator) evaluateInterpolatedBuiltinAssetArchive(x, s ast.Expr) (interface{}, bool) {
	v, b := e.evaluateExpr(s)
	if !b {
		return nil, false
	}

	createAssetArchiveF := e.lift(func(args ...interface{}) (interface{}, bool) {
		value, ok := args[0].(string)
		if !ok {
			return e.error(s, fmt.Sprintf("Argument to fn::* must be a string, got %v", reflect.TypeOf(args[0])))
		}

		switch x.(type) {
		case *ast.StringAssetExpr:
			return pulumi.NewStringAsset(value), true
		case *ast.FileArchiveExpr:
			return pulumi.NewFileArchive(value), true
		case *ast.FileAssetExpr:
			return pulumi.NewFileAsset(value), true
		case *ast.RemoteArchiveExpr:
			return pulumi.NewRemoteArchive(value), true
		case *ast.RemoteAssetExpr:
			return pulumi.NewRemoteAsset(value), true

		}
		return e.error(s, "unhandled expression")
	})

	return createAssetArchiveF(v)
}

func (e *programEvaluator) evaluateBuiltinReadFile(s *ast.ReadFileExpr) (interface{}, bool) {
	expr, ok := e.evaluateExpr(s.Path)
	if !ok {
		return nil, false
	}

	readFileF := e.lift(func(args ...interface{}) (interface{}, bool) {
		path, ok := args[0].(string)
		if !ok {
			return e.error(s.Path, fmt.Sprintf("Argument to fn::readFile must be a string, got %v", reflect.TypeOf(args[0])))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			e.error(s.Path, fmt.Sprintf("Error reading file at path %v: %v", path, err))
		}
		return string(data), true
	})

	return readFileF(expr)
}

func hasOutputs(v interface{}) bool {
	switch v := v.(type) {
	case pulumi.Output:
		return true
	case []interface{}:
		for _, e := range v {
			if hasOutputs(e) {
				return true
			}
		}
	case map[string]interface{}:
		for _, e := range v {
			if hasOutputs(e) {
				return true
			}
		}
	}
	return false
}

// lift wraps a function s.t. the function is called inside an Apply if any of its arguments contain Outputs.
// If none of the function's arguments contain Outputs, the function is called directly.
func (ctx *evalContext) lift(fn func(args ...interface{}) (interface{}, bool)) func(args ...interface{}) (interface{}, bool) {
	fnOrPoison := func(args ...interface{}) (interface{}, bool) {
		if p, ok := isPoisoned(args); ok {
			return p, true
		}
		return fn(args...)
	}
	return func(args ...interface{}) (interface{}, bool) {
		if hasOutputs(args) {
			return pulumi.All(args...).ApplyT(func(resolved []interface{}) (interface{}, error) {
				v, ok := fnOrPoison(resolved...)
				if !ok {
					// TODO: ensure that these appear in CLI
					return v, fmt.Errorf("runtime error")
				}
				return v, nil
			}), true
		}
		return fnOrPoison(args...)
	}
}

// untypedArgs is an untyped interface for a bag of properties.
type untypedArgs map[string]interface{}

func (untypedArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]interface{})(nil)).Elem()
}

func typeString(v interface{}) string {
	if v == nil {
		return "nil"
	}

	switch v.(type) {
	case bool:
		return "a boolean"
	case int:
		return "an integer"
	case float64:
		return "a number"
	case string:
		return "a string"
	case []interface{}:
		return "a list"
	case map[string]interface{}:
		return "an object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func listStrings(v *ast.StringListDecl) []string {
	a := make([]string, len(v.Elements))
	for i, s := range v.Elements {
		a[i] = s.Value
	}
	return a
}

// typeCheckerError indicates that Pulumi YAML found the wrong type for a situation that
// the type checker should have caught.
//
// typeCheckerError should not be used to indicate that a dynamic cast has failed.
type typeCheckerError struct {
	expected, found string
	location        ast.Expr
}

func (err typeCheckerError) Error() string {
	return fmt.Sprintf("%s must be a %s, instead got type %s",
		err.location.Syntax().String(), err.expected, err.found)
}

func (err typeCheckerError) Diag() *syntax.Diagnostic {
	const newIssue = "https://github.com/pulumi/pulumi-yaml/issues/new/choose"
	return ast.ExprError(err.location, err.Error(),
		"This indicates a bug in the Pulumi YAML type checker. "+
			"Please open an issue at "+newIssue)
}
