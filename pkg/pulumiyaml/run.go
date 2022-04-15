// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"bytes"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	ctypes "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/config"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
)

// MainTemplate is the assumed name of the JSON template file.
// TODO: would be nice to permit multiple files, but we'd need to know which is "main", and there's
//     no notion of "import" so we'd need to be a bit more clever. Might be nice to mimic e.g. Kustomize.
//     One idea is to hijack Pulumi.yaml's "main" directive and then just globally toposort the rest.
const MainTemplate = "Main"

// Load a template from the current working directory
func Load() (*ast.TemplateDecl, syntax.Diagnostics, error) {
	return load(true)
}

// Load a template from the current working directory.
func load(allowMain bool) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	// Read in the template file - search first for Main.json, then Main.yaml, then Pulumi.yaml.
	// The last of these will actually read the proram from the same Pulumi.yaml project file used by
	// Pulumi CLI, which now plays double duty, and allows a Pulumi deployment that uses a single file.
	var filename string
	var bs []byte
	if b, err := ioutil.ReadFile(MainTemplate + ".json"); err == nil {
		filename, bs = MainTemplate+".json", b
	} else if b, err := ioutil.ReadFile(MainTemplate + ".yaml"); err == nil {
		filename, bs = MainTemplate+".yaml", b
	} else if b, err := ioutil.ReadFile("Pulumi.yaml"); err == nil {
		filename, bs = "Pulumi.yaml", b
	} else {
		return nil, nil, fmt.Errorf("reading template %s: %w", MainTemplate, err)
	}

	return LoadYAMLBytes(filename, bs, allowMain)

}

// Load a template from the current working directory
func LoadFile(path string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	return LoadYAML(filepath.Base(path), f)
}

// LoadYAML decodes a YAML template from an io.Reader.
func LoadYAML(filename string, r io.Reader) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	return LoadYAMLBytes(filename, bytes, true)
}

// LoadYAMLBytes decodes a YAML template from a byte array.
func LoadYAMLBytes(filename string, source []byte, allowMain bool) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	var diags syntax.Diagnostics

	syn, sdiags := encoding.DecodeYAML(filename, yaml.NewDecoder(bytes.NewReader(source)), TagDecoder)
	diags.Extend(sdiags...)
	if sdiags.HasErrors() {
		return nil, diags, nil
	}

	t, tdiags := ast.ParseTemplate(source, syn)
	diags.Extend(tdiags...)

	mains := []syntax.ObjectPropertyDef{}
	for i := 0; i < syn.Len(); i++ {
		i := syn.Index(i)
		if i.Key.Value() == "main" {
			if allowMain {
				if _, ok := i.Value.(*syntax.StringNode); ok {
					mains = append(mains, i)
				} else {
					diags.Extend(syntax.NodeError(
						i.Value, "'main' key must be of type string",
						fmt.Sprintf("main is of type %T", i.Value)))
				}
			} else {
				diags.Extend(syntax.NodeError(i.Key, "'main' not allowed here", ""))
			}
		}
	}

	switch len(mains) {
	case 0:
		// No main specified, so we do nothing
	case 1:
		err := os.Chdir(mains[0].Value.(*syntax.StringNode).Value())
		// Other error messages won't be correct, so we exit early.
		if err != nil {
			diags.Extend(syntax.NodeError(mains[0].Value,
				fmt.Sprintf("Cannot set main directory: %s", err.Error()), ""))
			return nil, diags, err
		}
		te, d, e := load(false)
		diags.Extend(d...)
		return te, diags, e
	default:
		diags.Extend(syntax.NodeError(mains[1].Key, "Only 1 'main' key is allowed", ""))
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

// Run loads and evaluates a template using the given request/settings.
func Run(ctx *pulumi.Context) error {
	t, diags, err := Load()
	if err != nil {
		return multierror.Append(err, diags)
	}

	loader, err := NewPackageLoader()
	if err != nil {
		return err
	}
	defer loader.Close()

	// Now "evaluate" the template.
	return RunTemplate(ctx, t, loader)
}

// RunTemplate runs the evaluator against a template using the given request/settings.
func RunTemplate(ctx *pulumi.Context, t *ast.TemplateDecl, loader PackageLoader) error {
	runner := newRunner(ctx, t, loader)

	diags := TypeCheck(runner)
	if diags.HasErrors() {
		return diags
	}

	diags.Extend(runner.Evaluate()...)
	if diags.HasErrors() {
		return diags
	}
	return nil
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

type runner struct {
	ctx       *pulumi.Context
	t         *ast.TemplateDecl
	pkgLoader PackageLoader
	config    map[string]interface{}
	variables map[string]interface{}
	resources map[string]lateboundResource
	stackRefs map[string]*pulumi.StackReference

	sdiags syncDiags

	// Used to store sorted nodes. A non `nil` value indicates that the runner
	// is already setup for running.
	intermediates []graphNode
}

type evalContext struct {
	*runner

	root   interface{}
	sdiags syncDiags
}

func (ctx *evalContext) error(expr ast.Expr, summary string) (interface{}, bool) {
	diag := ast.ExprError(expr, summary, "")
	ctx.addDiag(diag)
	return nil, false
}

func (ctx *evalContext) addDiag(diag *syntax.Diagnostic) {
	ctx.sdiags.Extend(diag)
	ctx.runner.sdiags.Extend(diag)

	var buf bytes.Buffer
	w := ctx.t.NewDiagnosticWriter(&buf, 0, false)
	err := w.WriteDiagnostic(diag)
	if err != nil {
		err = ctx.ctx.Log.Error(fmt.Sprintf("internal error: %v", err), &pulumi.LogArgs{})
	} else {
		err = ctx.ctx.Log.Error(buf.String(), &pulumi.LogArgs{})
	}
	if err != nil {
		os.Stderr.Write([]byte(err.Error()))
	}
}

func (ctx *evalContext) errorf(expr ast.Expr, format string, a ...interface{}) (interface{}, bool) {
	return ctx.error(expr, fmt.Sprintf(format, a...))
}

func (r *runner) newContext(root interface{}) *evalContext {
	ctx := &evalContext{
		runner: r,
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
}

// lateboundCustomResourceState is a resource state that stores all computed outputs into a single
// MapOutput, so that we can access any output that was provided by the Pulumi engine without knowing
// up front the shape of the expected outputs.
type lateboundCustomResourceState struct {
	pulumi.CustomResourceState
	name    string
	Outputs pulumi.MapOutput `pulumi:""`
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
	return reflect.TypeOf((*lateboundResource)(nil)).Elem()
}

func (st *lateboundCustomResourceState) GetRawOutputs() pulumi.Output {
	return pulumi.InternalGetRawOutputs(&st.CustomResourceState.ResourceState)
}

type lateboundProviderResourceState struct {
	pulumi.ProviderResourceState
	name    string
	Outputs pulumi.MapOutput `pulumi:""`
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
	return reflect.TypeOf((*lateboundResource)(nil)).Elem()
}

func (st *lateboundProviderResourceState) GetRawOutputs() pulumi.Output {
	return pulumi.InternalGetRawOutputs(&st.CustomResourceState.ResourceState)
}

func newRunner(ctx *pulumi.Context, t *ast.TemplateDecl, p PackageLoader) *runner {
	return &runner{
		ctx:       ctx,
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
	EvalConfig(r *runner, node configNode) bool
	EvalVariable(r *runner, node variableNode) bool
	EvalResource(r *runner, node resourceNode) bool
	EvalOutput(r *runner, node ast.PropertyMapEntry) bool
}

type evaluator struct{}

func (evaluator) EvalConfig(r *runner, node configNode) bool {
	ctx := r.newContext(node)
	c, ok := ctx.registerConfig(node)
	if !ok {
		msg := fmt.Sprintf("Error registering config [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := r.ctx.Log.Error(msg, &pulumi.LogArgs{}) //nolint:errcheck
		if err != nil {
			return false
		}
	} else {
		r.config[node.Key.Value] = c
	}
	return true
}

func (evaluator) EvalVariable(r *runner, node variableNode) bool {
	ctx := r.newContext(node)
	value, ok := ctx.evaluateExpr(node.Value)
	if !ok {
		msg := fmt.Sprintf("Error registering variable [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := r.ctx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else {
		r.variables[node.Key.Value] = value
	}
	return true
}

func (evaluator) EvalResource(r *runner, node resourceNode) bool {
	ctx := r.newContext(node)
	res, ok := ctx.registerResource(node)
	if !ok {
		msg := fmt.Sprintf("Error registering resource [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := r.ctx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else {
		r.resources[node.Key.Value] = res
	}
	return true

}

func (evaluator) EvalOutput(r *runner, node ast.PropertyMapEntry) bool {
	ctx := r.newContext(node)
	out, ok := ctx.registerOutput(node)
	if !ok {
		msg := fmt.Sprintf("Error registering output [%v]: %v", node.Key.Value, ctx.sdiags.Error())
		err := r.ctx.Log.Error(msg, &pulumi.LogArgs{})
		if err != nil {
			return false
		}
	} else {
		r.ctx.Export(node.Key.Value, out)
	}
	return true
}

func (r *runner) Evaluate() syntax.Diagnostics {
	return r.Run(evaluator{})
}

func (r *runner) ensureSetup() {
	if r.intermediates == nil {
		r.intermediates = []graphNode{}
		cwd, err := os.Getwd()
		if err != nil {
			r.sdiags.Extend(syntax.Error(nil, err.Error(), ""))
			return
		}
		r.variables[PulumiVarName] = map[string]interface{}{
			"cwd":     cwd,
			"project": r.ctx.Project(),
			"stack":   r.ctx.Stack(),
		}

		// Topologically sort the intermediates based on implicit and explicit dependencies
		intermediates, rdiags := topologicallySortedResources(r.t)
		r.sdiags.Extend(rdiags...)
		if rdiags.HasErrors() {
			return
		}
		if intermediates != nil {
			r.intermediates = intermediates
		}
	}
}

func (r *runner) Run(e Evaluator) syntax.Diagnostics {
	r.ensureSetup()
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
			err := r.ctx.Log.Debug(fmt.Sprintf("Registering config [%v]", kvp.Key.Value), &pulumi.LogArgs{})
			if err != nil {
				return returnDiags()
			}
			if !e.EvalConfig(r, kvp) {
				return returnDiags()
			}
		case variableNode:
			err := r.ctx.Log.Debug(fmt.Sprintf("Registering variable [%v]", kvp.Key.Value), &pulumi.LogArgs{})
			if err != nil {
				return returnDiags()
			}
			if !e.EvalVariable(r, kvp) {
				return returnDiags()
			}
		case resourceNode:
			err := r.ctx.Log.Debug(fmt.Sprintf("Registering resource [%v]", kvp.Key.Value), &pulumi.LogArgs{})
			if err != nil {
				return returnDiags()
			}
			if !e.EvalResource(r, kvp) {
				return returnDiags()
			}
		}
	}

	for _, kvp := range r.t.Outputs.Entries {
		if !e.EvalOutput(r, kvp) {
			return returnDiags()
		}
	}

	return returnDiags()
}

func (ctx *evalContext) registerConfig(intm configNode) (interface{}, bool) {
	k, c := intm.Key.Value, intm.Value

	// If we implement global type checking, the type of configuration variables
	// can be inferred and this requirement relaxed.
	if c.Type == nil && c.Default == nil {
		return ctx.errorf(intm.Key, "unable to infer type: either 'default' or 'type' is required")
	}

	var defaultValue interface{}
	var expectedType ctypes.Type
	if c.Default != nil {
		d, ok := ctx.evaluateExpr(c.Default)
		if !ok {
			return nil, false
		}
		defaultValue = d
		switch d := d.(type) {
		case string:
			expectedType = ctypes.String
		case float64, int:
			expectedType = ctypes.Number
		case []interface{}:
			if len(d) == 0 && c.Type == nil {
				return ctx.errorf(c.Default,
					"unable to infer type: cannot infer type of empty list, please specify type")
			}
			switch d[0].(type) {
			case string:
				expectedType = ctypes.StringList
			case int, float64:
				expectedType = ctypes.NumberList
			case bool:
				expectedType = ctypes.BooleanList
			}
			for i := 1; i < len(d); i++ {
				if reflect.TypeOf(d[i-1]) != reflect.TypeOf(d[i]) {
					return ctx.errorf(c.Default,
						"heterogeneous typed lists are not allowed: found types %T and %T", d[i-1], d[i])
				}
			}
		case []int, []float64:
			expectedType = ctypes.NumberList
		default:
			return ctx.errorf(c.Default,
				"unexpected configuration type '%T': valid types are %s",
				d, ctypes.ConfigTypes)
		}
	}

	if c.Type != nil {
		t, ok := ctypes.Parse(c.Type.Value)
		if !ok {
			return ctx.errorf(c.Type,
				"unexpected configuration type '%s': valid types are %s",
				c.Type.Value, ctypes.ConfigTypes)
		}

		// We have both a default value and a explicit type. Make sure they
		// agree.
		if ctypes.IsValidType(expectedType) && t != expectedType {
			return ctx.errorf(intm.Key,
				"type mismatch: default value of type %s but type %s was specified",
				expectedType, t)
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
	isSecretInConfig := ctx.ctx.IsConfigSecret(ctx.ctx.Project() + ":" + k)

	if isSecretInConfig && c.Secret != nil && !c.Secret.Value {
		return ctx.error(c.Secret,
			"Cannot mark a configuration value as not secret"+
				" if the associated config value is secret")
	}

	var v interface{}
	var err error
	switch expectedType {
	case ctypes.String:
		if isSecretInConfig {
			v, err = config.TrySecret(ctx.ctx, k)
		} else {
			v, err = config.Try(ctx.ctx, k)
		}
	case ctypes.Number:
		if isSecretInConfig {
			v, err = config.TrySecretFloat64(ctx.ctx, k)
		} else {
			v, err = config.TryFloat64(ctx.ctx, k)
		}
	case ctypes.NumberList:
		var arr []float64
		if isSecretInConfig {
			v, err = config.TrySecretObject(ctx.ctx, k, &arr)
		} else {
			err = config.TryObject(ctx.ctx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	case ctypes.StringList:
		var arr []string
		if isSecretInConfig {
			v, err = config.TrySecretObject(ctx.ctx, k, &arr)
		} else {
			err = config.TryObject(ctx.ctx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	case ctypes.BooleanList:
		var arr []bool
		if isSecretInConfig {
			v, err = config.TrySecretObject(ctx.ctx, k, &arr)
		} else {
			err = config.TryObject(ctx.ctx, k, &arr)
			if err == nil {
				v = arr
			}
		}
	}

	if errors.Is(err, config.ErrMissingVar) && defaultValue != nil {
		v = defaultValue
	} else if err != nil {
		return ctx.errorf(intm.Key, err.Error())
	}

	// The value was marked secret in the configuration section, but in the
	// config section. We need to wrap it in `pulumi.ToSecret`.
	if (c.Secret != nil && c.Secret.Value) && !isSecretInConfig {
		v = pulumi.ToSecret(v)
	}
	return v, true
}

func (ctx *evalContext) registerResource(kvp resourceNode) (lateboundResource, bool) {
	k, v := kvp.Key.Value, kvp.Value

	// Read the properties and then evaluate them in case there are expressions contained inside.
	props := make(map[string]interface{})
	overallOk := true

	pkg, typ, err := ResolveResource(ctx.pkgLoader, v.Type.Value)
	if err != nil {
		ctx.error(v.Type, fmt.Sprintf("error resolving type of resource %v: %v", kvp.Key.Value, err))
		overallOk = false
	}

	for _, kvp := range v.Properties.Entries {
		vv, ok := ctx.evaluateExpr(kvp.Value)
		if !ok {
			overallOk = false
		}
		props[kvp.Key.Value] = vv
	}

	var opts []pulumi.ResourceOption
	if v.Options.AdditionalSecretOutputs != nil {
		opts = append(opts, pulumi.AdditionalSecretOutputs(listStrings(v.Options.AdditionalSecretOutputs)))
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
		dependOnOpt, ok := ctx.evaluateResourceListValuedOption(v.Options.DependsOn, "dependsOn")
		if ok {
			var dependsOn []pulumi.Resource
			for _, r := range dependOnOpt {
				dependsOn = append(dependsOn, r.CustomResource())
			}
			opts = append(opts, pulumi.DependsOn(dependsOn))
		} else {
			overallOk = false
		}
	}
	if v.Options.IgnoreChanges != nil {
		opts = append(opts, pulumi.IgnoreChanges(listStrings(v.Options.IgnoreChanges)))
	}
	if v.Options.Parent != nil {
		parentOpt, ok := ctx.evaluateResourceValuedOption(v.Options.Parent, "parent")
		if ok {
			opts = append(opts, pulumi.Parent(parentOpt.CustomResource()))
		} else {
			overallOk = false
		}
	}
	if v.Options.Protect != nil {
		opts = append(opts, pulumi.Protect(v.Options.Protect.Value))
	}
	if v.Options.Provider != nil {
		providerOpt, ok := ctx.evaluateResourceValuedOption(v.Options.Provider, "provider")
		if ok {
			provider := providerOpt.ProviderResource()
			if provider == nil {
				ctx.error(v.Options.Provider, fmt.Sprintf("resource passed as Provider was not a provider resource '%s'", providerOpt))
			} else {
				opts = append(opts, pulumi.Provider(provider))
			}
		} else {
			overallOk = false
		}
	}
	if v.Options.Providers != nil {
		dependOnOpt, ok := ctx.evaluateResourceListValuedOption(v.Options.Providers, "providers")
		if ok {
			var providers []pulumi.ProviderResource
			for _, r := range dependOnOpt {
				provider := r.ProviderResource()
				if provider == nil {
					ctx.error(v.Options.Provider, fmt.Sprintf("resource passed as provider was not a provider resource '%s'", r))
				} else {
					providers = append(providers, provider)
				}
			}
			opts = append(opts, pulumi.Providers(providers...))
		} else {
			overallOk = false
		}
	}
	if v.Options.Version != nil {
		opts = append(opts, pulumi.Version(v.Options.Version.Value))
	}
	if v.Options.PluginDownloadURL != nil {
		opts = append(opts, pulumi.PluginDownloadURL(v.Options.PluginDownloadURL.Value))
	}
	if v.Options.ReplaceOnChanges != nil {
		opts = append(opts, pulumi.ReplaceOnChanges(listStrings(v.Options.ReplaceOnChanges)))
	}

	// Create either a latebound custom resource or latebound provider resource depending on
	// whether the type token indicates a special provider type.
	var state lateboundResource
	var res pulumi.Resource
	isProvider := false
	if strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
		r := lateboundProviderResourceState{name: k}
		state = &r
		res = &r
		isProvider = true
	} else {
		r := lateboundCustomResourceState{name: k}
		state = &r
		res = &r
	}

	if !overallOk || ctx.sdiags.HasErrors() {
		return nil, false
	}

	isComponent := false
	if !isProvider {
		result, err := pkg.IsComponent(typ)
		if err != nil {
			ctx.error(v.Type, "unable to resolve type")
			return nil, false
		}
		isComponent = result
	}

	constants := pkg.ResourceConstants(typ)
	for k, v := range constants {
		props[k] = v
	}

	// Now register the resulting resource with the engine.
	if isComponent {
		err = ctx.ctx.RegisterRemoteComponentResource(string(typ), k, untypedArgs(props), res, opts...)
	} else {
		err = ctx.ctx.RegisterResource(string(typ), k, untypedArgs(props), res, opts...)
	}
	if err != nil {
		ctx.error(kvp.Key, err.Error())
		return nil, false
	}

	return state, true
}

func (ctx *evalContext) evaluateResourceListValuedOption(optionExpr ast.Expr, key string) ([]lateboundResource, bool) {
	value, ok := ctx.evaluateExpr(optionExpr)
	if !ok {
		return nil, false
	}
	if hasOutputs(value) {
		ctx.error(optionExpr, fmt.Sprintf("resource option %v value must be a list of resource, not an output", key))
		return nil, false
	}
	dependencies, ok := value.([]interface{})
	if !ok {
		ctx.error(optionExpr, fmt.Sprintf("resource option %v value must be a list of resources", key))
		return nil, false
	}
	var resources []lateboundResource
	for _, dep := range dependencies {
		res, err := asResource(dep)
		if err != nil {
			ctx.error(optionExpr, err.Error())
			continue
		}
		resources = append(resources, res)
	}
	return resources, true
}

func (ctx *evalContext) evaluateResourceValuedOption(optionExpr ast.Expr, key string) (lateboundResource, bool) {
	value, ok := ctx.evaluateExpr(optionExpr)
	if !ok {
		return nil, false
	}
	if hasOutputs(value) {
		ctx.error(optionExpr, "resource cannot be an output")
		return nil, false
	}
	res, err := asResource(value)
	if err != nil {
		ctx.error(optionExpr, err.Error())
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

func (ctx *evalContext) registerOutput(kvp ast.PropertyMapEntry) (pulumi.Input, bool) {
	out, ok := ctx.evaluateExpr(kvp.Value)
	if !ok {
		return nil, false
	}

	switch res := out.(type) {
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
//
func (ctx *evalContext) evaluateExpr(x ast.Expr) (interface{}, bool) {
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
		return ctx.evaluateList(x)
	case *ast.ObjectExpr:
		return ctx.evaluateObject(x, map[string]interface{}{}, x.Entries)
	case *ast.InterpolateExpr:
		return ctx.evaluateInterpolate(x)
	case *ast.SymbolExpr:
		return ctx.evaluatePropertyAccess(x, x.Property)
	case *ast.InvokeExpr:
		return ctx.evaluateBuiltinInvoke(x)
	case *ast.JoinExpr:
		return ctx.evaluateBuiltinJoin(x)
	case *ast.SplitExpr:
		return ctx.evaluateBuiltinSplit(x)
	case *ast.ToJSONExpr:
		return ctx.evaluateBuiltinToJSON(x)
	case *ast.SelectExpr:
		return ctx.evaluateBuiltinSelect(x)
	case *ast.ToBase64Expr:
		return ctx.evaluateBuiltinToBase64(x)
	case *ast.FileAssetExpr:
		return pulumi.NewFileAsset(x.Source.Value), true
	case *ast.StringAssetExpr:
		return pulumi.NewStringAsset(x.Source.Value), true
	case *ast.RemoteAssetExpr:
		return pulumi.NewRemoteAsset(x.Source.Value), true
	case *ast.FileArchiveExpr:
		return pulumi.NewFileArchive(x.Source.Value), true
	case *ast.RemoteArchiveExpr:
		return pulumi.NewRemoteArchive(x.Source.Value), true
	case *ast.AssetArchiveExpr:
		return ctx.evaluateBuiltinAssetArchive(x)
	case *ast.StackReferenceExpr:
		return ctx.evaluateBuiltinStackReference(x)
	default:
		panic(fmt.Sprintf("fatal: invalid expr type %v", reflect.TypeOf(x)))
	}
}

func (ctx *evalContext) evaluateList(x *ast.ListExpr) (interface{}, bool) {
	xs := make([]interface{}, len(x.Elements))
	for i, e := range x.Elements {
		ev, ok := ctx.evaluateExpr(e)
		if !ok {
			return nil, false
		}
		xs[i] = ev
	}
	return xs, true
}

func (ctx *evalContext) evaluateObject(x *ast.ObjectExpr, m map[string]interface{}, entries []ast.ObjectProperty) (interface{}, bool) {
	if len(entries) == 0 {
		return m, true
	}

	kvp := entries[0]

	kv, ok := ctx.evaluateExpr(kvp.Key)
	if !ok {
		return nil, false
	}

	if o, ok := kv.(pulumi.Output); ok {
		return o.ApplyT(func(kv interface{}) (interface{}, error) {
			v, ok := ctx.continueObject(x, m, kvp, kv, entries)
			if !ok {
				return nil, fmt.Errorf("runtime error")
			}
			return v, nil
		}), true
	}

	return ctx.continueObject(x, m, kvp, kv, entries)
}

func (ctx *evalContext) continueObject(x *ast.ObjectExpr, m map[string]interface{}, kvp ast.ObjectProperty, kv interface{}, entries []ast.ObjectProperty) (interface{}, bool) {
	k, ok := kv.(string)
	if !ok {
		return ctx.error(kvp.Key, fmt.Sprintf("object key must evaluate to a string, not %v", typeString(kv)))
	}

	v, ok := ctx.evaluateExpr(kvp.Value)
	if !ok {
		return nil, false
	}

	m[k] = v
	return ctx.evaluateObject(x, m, entries[1:])
}

func (ctx *evalContext) evaluateInterpolate(x *ast.InterpolateExpr) (interface{}, bool) {
	return ctx.evaluateInterpolations(x, &strings.Builder{}, x.Parts)
}

func (ctx *evalContext) evaluateInterpolations(x *ast.InterpolateExpr, b *strings.Builder, parts []ast.Interpolation) (interface{}, bool) {
	for ; len(parts) > 0; parts = parts[1:] {
		i := parts[0]
		b.WriteString(i.Text)

		if i.Value != nil {
			p, ok := ctx.evaluatePropertyAccess(x, i.Value)
			if !ok {
				return nil, false
			}

			if o, ok := p.(pulumi.Output); ok {
				return o.ApplyT(func(v interface{}) (interface{}, error) {
					fmt.Fprintf(b, "%v", v)
					v, ok := ctx.evaluateInterpolations(x, b, parts[1:])
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
	return pulumi.UnsafeUnknownOutput()
}

// evaluatePropertyAccess evaluates interpolation expressions, `${foo.bar[baz]}`. The first item in
// the property access list is the head, and must be an identifier for a resource, config, or
// variable. The tail of property accessors are either: `.foo` string literal property names or
// `[42]` numeric literal property subscripts.
func (ctx *evalContext) evaluatePropertyAccess(expr ast.Expr, access *ast.PropertyAccess) (interface{}, bool) {
	resourceName := access.Accessors[0].(*ast.PropertyName).Name

	var receiver interface{}
	if res, ok := ctx.resources[resourceName]; ok {
		receiver = res
	} else if p, ok := ctx.config[resourceName]; ok {
		receiver = p
	} else if v, ok := ctx.variables[resourceName]; ok {
		receiver = v
	} else {
		return ctx.error(expr, fmt.Sprintf("resource or variable named %s could not be found", resourceName))
	}

	return ctx.evaluatePropertyAccessTail(expr, receiver, access.Accessors[1:])
}

func (ctx *evalContext) isVariableContext() bool {
	_, ok := ctx.root.(variableNode)

	return ok
}

func (ctx *evalContext) evaluatePropertyAccessTail(expr ast.Expr, receiver interface{}, accessors []ast.PropertyAccessor) (interface{}, bool) {
	var evaluateAccessF func(args ...interface{}) (interface{}, bool)
	evaluateAccessF = ctx.lift(func(args ...interface{}) (interface{}, bool) {
		receiver := args[0]
		accessors := args[1].([]ast.PropertyAccessor)
	Loop:
		for {
			switch x := receiver.(type) {
			case lateboundResource:
				// Peak ahead at the next accessor to implement .urn and .id:
				if len(accessors) >= 1 {
					sub, ok := accessors[0].(*ast.PropertyName)
					if ok && sub.Name == "id" {
						return x.CustomResource().ID().ToStringOutput(), true
					} else if ok && sub.Name == "urn" {
						return x.CustomResource().URN().ToStringOutput(), true
					}
					return evaluateAccessF(x.GetRawOutputs(), accessors)
				}
				return x, true
			case resource.PropertyMap:
				if len(accessors) == 0 {
					if !ctx.isVariableContext() {
						if x.ContainsUnknowns() {
							return unknownOutput(), true
						}
						receiver = x.Mappable()
					}
					break Loop
				}
				var k string
				switch a := accessors[0].(type) {
				case *ast.PropertyName:
					k = a.Name
				case *ast.PropertySubscript:
					s, ok := a.Index.(string)
					if !ok {
						return ctx.error(expr, "cannot access an object property using an integer index")
					}
					k = s
				}
				prop, ok := x[resource.PropertyKey(k)]
				if x.ContainsUnknowns() && !ok {
					return unknownOutput(), true
				} else if !ok {
					receiver = nil
				} else {
					receiver = prop
				}
				accessors = accessors[1:]
			case resource.PropertyValue:
				switch {
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
					asset := x.AssetValue()
					switch {
					case asset.IsPath():
						receiver = pulumi.NewFileAsset(asset.Path)
					case asset.IsText():
						receiver = pulumi.NewStringAsset(asset.Text)
					case asset.IsURI():
						receiver = pulumi.NewRemoteAsset(asset.URI)
					}
				case x.IsArchive():
					archive := x.ArchiveValue()
					switch {
					case archive.IsPath():
						receiver = pulumi.NewFileArchive(archive.Path)
					case archive.IsURI():
						receiver = pulumi.NewRemoteArchive(archive.URI)
					case archive.IsAssets():
						return x.AssetValue(), true
					}
				case x.IsResourceReference():
					return x.ResourceReferenceValue(), true
				default:
					receiver = x.V
				}
			case []resource.PropertyValue:
				if len(accessors) == 0 {
					if !ctx.isVariableContext() {
						if resource.NewArrayProperty(x).ContainsUnknowns() {
							return unknownOutput(), true
						}
						receiver = resource.NewArrayProperty(x).Mappable()
					}
					break Loop
				}
				sub, ok := accessors[0].(*ast.PropertySubscript)
				if !ok {
					return ctx.error(expr, "cannot access a list element using a property name")
				}
				index, ok := sub.Index.(int)
				if !ok {
					return ctx.error(expr, "cannot access a list element using a property name")
				}
				if index < 0 || index >= len(x) {
					return ctx.error(expr, fmt.Sprintf("list index %v out-of-bounds for list of length %v", index, len(x)))
				}
				receiver = x[index]
				accessors = accessors[1:]
			case []interface{}, []string, []int, []float64, []bool:
				if len(accessors) == 0 {
					break Loop
				}
				sub, ok := accessors[0].(*ast.PropertySubscript)
				if !ok {
					return ctx.error(expr, "cannot access a list element using a property name")
				}
				index, ok := sub.Index.(int)
				if !ok {
					return ctx.error(expr, "cannot access a list element using a property name")
				}
				reflx := reflect.ValueOf(x)
				length := reflx.Len()
				if index < 0 || index >= length {
					return ctx.error(expr, fmt.Sprintf("list index %v out-of-bounds for list of length %v", index, length))
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
						return ctx.error(expr, "cannot access an object property using an integer index")
					}
					k = s
				}
				receiver = x[k]
				accessors = accessors[1:]
			default:
				if len(accessors) == 0 {
					break Loop
				}
				return ctx.error(expr, fmt.Sprintf("receiver must be a list or object, not %v", typeString(receiver)))
			}
		}
		return receiver, true
	})

	return evaluateAccessF(receiver, accessors)
}

// evaluateBuiltinInvoke evaluates the "Invoke" builtin, which enables templates to invoke arbitrary
// data source functions, to fetch information like the current availability zone, lookup AMIs, etc.
func (ctx *evalContext) evaluateBuiltinInvoke(t *ast.InvokeExpr) (interface{}, bool) {
	args, ok := ctx.evaluateExpr(t.CallArgs)
	if !ok {
		return nil, false
	}

	performInvoke := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		// At this point, we've got a function to invoke and some parameters! Invoke away.
		result := map[string]interface{}{}
		_, functionName, err := ResolveFunction(ctx.pkgLoader, t.Token.Value)
		if err != nil {
			return ctx.error(t, err.Error())
		}

		if err := ctx.ctx.Invoke(string(functionName), args[0], &result); err != nil {
			return ctx.error(t, err.Error())
		}

		if t.Return.GetValue() == "" {
			return result, true
		}

		retv, ok := result[t.Return.Value]
		if !ok {
			ctx.error(t.Return, fmt.Sprintf("Unable to evaluate result[%v], result is: %+v", t.Return.Value, t.Return))
			return ctx.error(t.Return, fmt.Sprintf("Fn::Invoke of %s did not contain a property '%s' in the returned value", t.Token.Value, t.Return.Value))
		}
		return retv, true
	})
	return performInvoke(args)
}

func (ctx *evalContext) evaluateBuiltinJoin(v *ast.JoinExpr) (interface{}, bool) {
	delim, ok := ctx.evaluateExpr(v.Delimiter)
	if !ok {
		return nil, false
	}

	overallOk := true

	parts := make([]interface{}, len(v.Values.Elements))
	for i, e := range v.Values.Elements {
		part, ok := ctx.evaluateExpr(e)
		if !ok {
			overallOk = false
		}
		parts[i] = part
	}

	if !overallOk {
		return nil, false
	}

	join := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		delim, parts := args[0], args[1].([]interface{})

		if delim == nil {
			delim = ""
		}
		delimStr, ok := delim.(string)
		if !ok {
			ctx.error(v.Delimiter, fmt.Sprintf("delimiter must be a string, not %v", typeString(delimStr)))
		}

		overallOk := true

		strs := make([]string, len(parts))
		for i, p := range parts {
			str, ok := p.(string)
			if !ok {
				ctx.error(v.Values.Elements[i], fmt.Sprintf("element must be a string, not %v", typeString(p)))
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
	return join(delim, parts)
}

func (ctx *evalContext) evaluateBuiltinSplit(v *ast.SplitExpr) (interface{}, bool) {
	delimiter, delimOk := ctx.evaluateExpr(v.Delimiter)
	source, sourceOk := ctx.evaluateExpr(v.Source)
	if !delimOk || !sourceOk {
		return nil, false
	}

	split := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		d, delimOk := args[0].(string)
		if !delimOk {
			ctx.error(v.Delimiter, fmt.Sprintf("Must be a string, not %v", typeString(d)))
		}
		s, sourceOk := args[1].(string)
		if !sourceOk {
			ctx.error(v.Source, fmt.Sprintf("Must be a string, not %v", typeString(s)))
		}
		if !delimOk || !sourceOk {
			return nil, false
		}
		return strings.Split(s, d), true
	})
	return split(delimiter, source)
}

func (ctx *evalContext) evaluateBuiltinToJSON(v *ast.ToJSONExpr) (interface{}, bool) {
	value, ok := ctx.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}

	toJSON := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		b, err := json.Marshal(args[0])
		if err != nil {
			ctx.error(v, fmt.Sprintf("failed to encode JSON: %v", err))
			return "", false
		}
		return string(b), true
	})
	return toJSON(value)
}

func (ctx *evalContext) evaluateBuiltinSelect(v *ast.SelectExpr) (interface{}, bool) {
	index, ok := ctx.evaluateExpr(v.Index)
	if !ok {
		return nil, false
	}
	values, ok := ctx.evaluateExpr(v.Values)
	if !ok {
		return nil, false
	}

	selectFn := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		indexArg := args[0]
		elemsArg := args[1]

		index, ok := indexArg.(float64)
		if !ok {
			return ctx.error(v.Index, fmt.Sprintf("index must be a number, not %v", typeString(indexArg)))
		}
		if float64(int(index)) != index || int(index) < 0 {
			// Cannot be a valid index, so we error
			f := strconv.FormatFloat(index, 'f', -1, 64) // Manual formatting is so -3 does not get formatted as -3.0
			return ctx.error(v.Index, fmt.Sprintf("index must be a positive integral, not %s", f))
		}
		intIndex := int(index)

		return ctx.evaluatePropertyAccessTail(v.Values, elemsArg, []ast.PropertyAccessor{&ast.PropertySubscript{Index: intIndex}})
	})
	return selectFn(index, values)
}

func (ctx *evalContext) evaluateBuiltinToBase64(v *ast.ToBase64Expr) (interface{}, bool) {
	str, ok := ctx.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}
	toBase64 := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		s, ok := args[0].(string)
		if !ok {
			return nil, false
		}
		return b64.StdEncoding.EncodeToString([]byte(s)), true
	})
	return toBase64(str)
}

func (ctx *evalContext) evaluateBuiltinAssetArchive(v *ast.AssetArchiveExpr) (interface{}, bool) {
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
		assetOrArchive, ok := ctx.evaluateExpr(v)
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

func (ctx *evalContext) evaluateBuiltinStackReference(v *ast.StackReferenceExpr) (interface{}, bool) {
	stackRef, ok := ctx.stackRefs[v.StackName.Value]
	if !ok {
		var err error
		stackRef, err = pulumi.NewStackReference(ctx.ctx, v.StackName.Value, &pulumi.StackReferenceArgs{})
		if err != nil {
			return ctx.error(v.StackName, err.Error())
		}
		ctx.stackRefs[v.StackName.Value] = stackRef
	}

	property, ok := ctx.evaluateExpr(v.PropertyName)
	if !ok {
		return nil, false
	}

	propertyStringOutput := pulumi.ToOutput(property).ApplyT(func(n interface{}) (string, error) {
		s, ok := n.(string)
		if !ok {
			ctx.error(v.PropertyName,
				fmt.Sprintf("expected property name argument to Fn::StackReference to be a string, got %v", typeString(n)),
			)
		}
		return s, nil
	}).(pulumi.StringOutput)

	return stackRef.GetOutput(propertyStringOutput), true
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
	return func(args ...interface{}) (interface{}, bool) {
		if hasOutputs(args) {
			return pulumi.All(args...).ApplyT(func(resolved []interface{}) (interface{}, error) {
				v, ok := fn(resolved...)
				if !ok {
					// TODO: ensure that these appear in CLI
					return v, fmt.Errorf("runtime error")
				}
				return v, nil
			}), true
		}
		return fn(args...)
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
