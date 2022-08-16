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
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
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
// no notion of "import" so we'd need to be a bit more clever. Might be nice to mimic e.g. Kustomize.
// One idea is to hijack Pulumi.yaml's "main" directive and then just globally toposort the rest.
const MainTemplate = "Main"

// Load a template from the current working directory
func Load() (*ast.TemplateDecl, syntax.Diagnostics, error) {
	return LoadDir(".")
}

func LoadFromCompiler(compiler string, workingDirectory string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
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

	return template, tdiags, err
}

// Load a template from the current working directory.
func LoadDir(cwd string) (*ast.TemplateDecl, syntax.Diagnostics, error) {
	// Read in the template file - search first for Main.json, then Main.yaml, then Pulumi.yaml.
	// The last of these will actually read the proram from the same Pulumi.yaml project file used by
	// Pulumi CLI, which now plays double duty, and allows a Pulumi deployment that uses a single file.
	var filename string
	var bs []byte
	if b, err := ioutil.ReadFile(filepath.Join(cwd, MainTemplate+".json")); err == nil {
		filename, bs = MainTemplate+".json", b
	} else if b, err := ioutil.ReadFile(filepath.Join(cwd, MainTemplate+".yaml")); err == nil {
		filename, bs = MainTemplate+".yaml", b
	} else if b, err := ioutil.ReadFile(filepath.Join(cwd, "Pulumi.yaml")); err == nil {
		filename, bs = "Pulumi.yaml", b
	} else {
		return nil, nil, fmt.Errorf("reading template %s: %w", MainTemplate, err)
	}

	return LoadYAMLBytes(filename, bs)
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

func setDefaultProviders(ctx *analyzeContext) error {
	defaultProviderInfoMap := make(map[string]*providerInfo)
	for _, resource := range ctx.t.Resources.Entries {
		v := resource.Value
		// check if this is a provider resource
		if strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
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
			return errors.New("cannot set defaultProvider on non-provider resource")
		}
	}

	checkOptions := func(opts ast.ProviderOpts, providerName string) bool {
		if opts.HasProvider() {
			if opts.GetVersion() != nil {
				ctx.errorf(opts.GetVersion(), "Version conflicts with the default provider version.")
			}
			if opts.GetPluginDownloadURL() != nil {
				ctx.errorf(opts.GetPluginDownloadURL(), "PluginDownloadURL conflicts with the default provider URL.")
			}

			expr, diags := ast.VariableSubstitution(providerName)
			if diags.HasErrors() {
				ctx.sdiags.diags = append(ctx.sdiags.diags, diags...)
				return false
			}
			opts.SetProvider(expr)
		}
	}

	// Set roots
	walker := walker{
		VisitResource: func(node resourceNode) bool {
			_, v := node.Key.Value, node.Value
			if strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
				return true
			}
			pkgName := strings.Split(v.Type.Value, ":")[0]

			if _, ok := defaultProviderInfoMap[pkgName]; !ok {
				return true
			}
			defaultProviderInfo := defaultProviderInfoMap[pkgName]

			return checkOptions(&v.Options, defaultProviderInfo.providerName.Value)
		},
		VisitExpr: func(e ast.Expr) bool {
			switch t := e.(type) {
			case *ast.InvokeExpr:
				pkgName := strings.Split(t.Token.Value, ":")[0]
				if _, ok := defaultProviderInfoMap[pkgName]; !ok {
					return true
				}
				defaultProviderInfo := defaultProviderInfoMap[pkgName]

				return checkOptions(&t.CallOpts, defaultProviderInfo.providerName.Value)
			}
			return true
		},
		VisitVariable: func(ctx *evalContext, node variableNode) bool {
			return true
		},
		VisitConfig: func(ctx *evalContext, node configNode) bool {
			return true
		},
		VisitOutput: func(node ast.PropertyMapEntry) bool {
			return true
		},
	}
	Run(ctx, walker)

	if ctx.sdiags.HasErrors() {
		return &ctx.sdiags
	}
	return nil
}

// RunTemplate runs the evaluator against a template using the given request/settings.
func RunTemplate(ctx *pulumi.Context, t *ast.TemplateDecl, loader PackageLoader) error {
	evalCtx := newContext(ctx, t, loader)

	err := setDefaultProviders(evalCtx)
	if err != nil {
		return err
	}

	_, diags := TypeCheck(evalCtx)
	if diags.HasErrors() {
		return diags
	}

	diags.Extend(Evaluate(evalCtx)...)
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

type providerInfo struct {
	version           *ast.StringExpr
	pluginDownloadURL *ast.StringExpr
	providerName      *ast.StringExpr
}

type runner struct {
	ctx       *pulumi.Context
	t         *ast.TemplateDecl
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
	ctx       *pulumi.Context
	t         *ast.TemplateDecl
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

func (ctx *evalContext) error(expr ast.Expr, summary string) (interface{}, bool) {
	diag := ast.ExprError(expr, summary, "")
	ctx.addDiag(diag)
	return nil, false
}

func (ctx *evalContext) addDiag(diag *syntax.Diagnostic) {
	defer func() {
		ctx.sdiags.Extend(diag)
	}()

	var buf bytes.Buffer
	w := ctx.t.NewDiagnosticWriter(&buf, 0, false)
	err := w.WriteDiagnostic(diag.HCL())
	if err != nil {
		err = ctx.ctx.Log.Error(fmt.Sprintf("internal error: %v", err), &pulumi.LogArgs{})
	} else {
		s := buf.String()
		// We strip off the appropriate HCL error message, since it will be
		// added back on via the pulumi.Log framework.
		switch diag.Severity {
		case hcl.DiagWarning:
			s = strings.TrimPrefix(s, "Warning: ")
			err = ctx.ctx.Log.Warn(s, &pulumi.LogArgs{})
		default:
			s = strings.TrimPrefix(s, "Error: ")
			err = ctx.ctx.Log.Error(s, &pulumi.LogArgs{})
		}
	}
	if err != nil {
		os.Stderr.Write([]byte(err.Error()))
	} else {
		diag.Shown = true
	}
}

func (ctx *evalContext) errorf(expr ast.Expr, format string, a ...interface{}) (interface{}, bool) {
	return ctx.error(expr, fmt.Sprintf(format, a...))
}

func (ctx *evalContext) getPkgLoader() PackageLoader {
	return ctx.pkgLoader
}

func newContext(ctx *pulumi.Context, t *ast.TemplateDecl, p PackageLoader) *evalContext {
	return &evalContext{
		ctx:       ctx,
		t:         t,
		pkgLoader: p,
		sdiags:    syncDiags{},
	}
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
	EvalConfig(ctx *evalContext, node configNode) bool
	EvalVariable(ctx *evalContext, node variableNode) bool
	EvalResource(ctx *evalContext, node resourceNode) bool
	EvalOutput(ctx *evalContext, node ast.PropertyMapEntry) bool
}

type evaluator struct{}

func (evaluator) EvalConfig(ctx *evalContext, node configNode) bool {
	// TODO: refactor poison marker logic
	ctx.registerConfig(node)
	// if !ok {
	// 	r.config[node.Key.Value] = poisonMarker{}
	// 	msg := fmt.Sprintf("Error registering config [%v]: %v", node.Key.Value, ctx.sdiags.Error())
	// 	err := r.ctx.Log.Error(msg, &pulumi.LogArgs{}) //nolint:errcheck
	// 	if err != nil {
	// 		return false
	// 	}
	// } else {
	// 	r.config[node.Key.Value] = c
	// }
	return true
}

func (evaluator) EvalVariable(ctx *evalContext, node variableNode) bool {
	ctx.evaluateExpr(node.Value)
	// if !ok {
	// 	r.variables[node.Key.Value] = poisonMarker{}
	// 	msg := fmt.Sprintf("Error registering variable [%v]: %v", node.Key.Value, ctx.sdiags.Error())
	// 	err := r.ctx.Log.Error(msg, &pulumi.LogArgs{})
	// 	if err != nil {
	// 		return false
	// 	}
	// } else {
	// 	r.variables[node.Key.Value] = value
	// }
	return true
}

func (evaluator) EvalResource(ctx *evalContext, node resourceNode) bool {
	ctx.registerResource(node)
	// if !ok {
	// 	r.resources[node.Key.Value] = poisonMarker{}
	// 	msg := fmt.Sprintf("Error registering resource [%v]: %v", node.Key.Value, ctx.sdiags.Error())
	// 	err := r.ctx.Log.Error(msg, &pulumi.LogArgs{})
	// 	if err != nil {
	// 		return false
	// 	}
	// } else {
	// 	r.resources[node.Key.Value] = res
	// }
	return true

}

func (evaluator) EvalOutput(ctx *evalContext, node ast.PropertyMapEntry) bool {
	ctx.registerOutput(node)
	// if !ok {
	// 	msg := fmt.Sprintf("Error registering output [%v]: %v", node.Key.Value, ctx.sdiags.Error())
	// 	err := r.ctx.Log.Error(msg, &pulumi.LogArgs{})
	// 	if err != nil {
	// 		return false
	// 	}
	// } else if _, poisoned := out.(poisonMarker); !poisoned {
	// 	r.ctx.Export(node.Key.Value, out)
	// }
	return true
}

func Evaluate(ctx *evalContext) syntax.Diagnostics {
	return Run(ctx, evaluator{})
}

func ensureSetup(ctx *evalContext) {
	ctx.intermediates = []graphNode{}
	cwd, err := os.Getwd()
	if err != nil {
		ctx.sdiags.Extend(syntax.Error(nil, err.Error(), ""))
		return
	}
	ctx.variables[PulumiVarName] = map[string]interface{}{
		"cwd":     cwd,
		"project": ctx.ctx.Project(),
		"stack":   ctx.ctx.Stack(),
	}
	ctx.cwd = cwd

	// Topologically sort the intermediates based on implicit and explicit dependencies
	intermediates, rdiags := topologicallySortedResources(ctx.t)
	ctx.sdiags.Extend(rdiags...)
	if rdiags.HasErrors() {
		return
	}
	if intermediates != nil {
		ctx.intermediates = intermediates
	}
}

func Run(ctx *evalContext, e Evaluator) syntax.Diagnostics {
	ensureSetup(ctx)
	returnDiags := func() syntax.Diagnostics {
		ctx.sdiags.mutex.Lock()
		defer ctx.sdiags.mutex.Unlock()
		return ctx.sdiags.diags
	}
	if ctx.sdiags.HasErrors() {
		return returnDiags()
	}

	for _, kvp := range ctx.intermediates {
		switch kvp := kvp.(type) {
		case configNode:
			err := ctx.ctx.Log.Debug(fmt.Sprintf("Registering config [%v]", kvp.Key.Value), &pulumi.LogArgs{})
			if err != nil {
				return returnDiags()
			}
			if !e.EvalConfig(ctx, kvp) {
				return returnDiags()
			}
		case variableNode:
			err := ctx.ctx.Log.Debug(fmt.Sprintf("Registering variable [%v]", kvp.Key.Value), &pulumi.LogArgs{})
			if err != nil {
				return returnDiags()
			}
			if !e.EvalVariable(ctx, kvp) {
				return returnDiags()
			}
		case resourceNode:
			err := ctx.ctx.Log.Debug(fmt.Sprintf("Registering resource [%v]", kvp.Key.Value), &pulumi.LogArgs{})
			if err != nil {
				return returnDiags()
			}
			if !e.EvalResource(ctx, kvp) {
				return returnDiags()
			}
		}
	}

	for _, kvp := range ctx.t.Outputs.Entries {
		if !e.EvalOutput(ctx, kvp) {
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
		case bool:
			expectedType = ctypes.Boolean
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

	readIntoProperties := func(obj ast.PropertyMapDecl) (poisonMarker, bool) {
		for _, kvp := range obj.Entries {
			vv, ok := ctx.evaluateExpr(kvp.Value)
			if !ok {
				overallOk = false
			}
			if p, ok := vv.(poisonMarker); ok {
				return p, true
			}
			props[kvp.Key.Value] = vv
		}
		return poisonMarker{}, false
	}

	if p, isPoison := readIntoProperties(v.Properties); isPoison {
		return p, isPoison
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
	if v.Options.IgnoreChanges != nil {
		opts = append(opts, pulumi.IgnoreChanges(listStrings(v.Options.IgnoreChanges)))
	}
	if v.Options.Parent != nil {
		parentOpt, ok := ctx.evaluateResourceValuedOption(v.Options.Parent, "parent")
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
		opts = append(opts, pulumi.Protect(v.Options.Protect.Value))
	}
	if v.Options.Provider != nil {
		providerOpt, ok := ctx.evaluateResourceValuedOption(v.Options.Provider, "provider")
		if ok {
			if p, ok := providerOpt.(poisonMarker); ok {
				return p, true
			}
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
				if p, ok := r.(poisonMarker); ok {
					return p, true
				}
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
	if b := v.Options.RetainOnDelete; b != nil {
		opts = append(opts, pulumi.RetainOnDelete(b.Value))
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

	isRead := v.Get.Id != nil
	if isRead {
		contract.Assertf(len(props) == 0, "Failed to check that Properties cannot be specified with Get.State")
		p, isPoison := readIntoProperties(v.Get.State)
		if isPoison {
			return p, true
		}
	}

	// Now register the resulting resource with the engine.
	if isComponent {
		err = ctx.ctx.RegisterRemoteComponentResource(string(typ), k, untypedArgs(props), res, opts...)
	} else if isRead {
		s, ok := ctx.evaluateExpr(v.Get.Id)
		if !ok {
			ctx.error(v.Get.Id, "unable to evaluate get.id")
			return nil, false
		}
		if p, ok := s.(poisonMarker); ok {
			return p, true
		}
		id, ok := s.(string)
		if !ok {
			ctx.errorf(v.Get.Id, "get.id must be a prompt string, instead got type %T", s)
			return nil, false
		}
		err = ctx.ctx.ReadResource(string(typ), k, pulumi.ID(id), untypedArgs(props), res.(pulumi.CustomResource), opts...)
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
		var entries []ast.ObjectProperty
		if x != nil {
			entries = x.Entries
		}
		return ctx.evaluateObject(x, entries)
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
	case *ast.FromBase64Expr:
		return ctx.evaluateBuiltinFromBase64(x)
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
	case *ast.SecretExpr:
		return ctx.evaluateBuiltinSecret(x)
	case *ast.ReadFileExpr:
		return ctx.evaluateBuiltinReadFile(x)
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
		if p, ok := ev.(poisonMarker); ok {
			return p, true
		}
		xs[i] = ev
	}
	return xs, true
}

func (ctx *evalContext) evaluateObject(x *ast.ObjectExpr, entries []ast.ObjectProperty) (interface{}, bool) {
	if len(entries) == 0 {
		return map[string]interface{}{}, true
	}

	allOk := true
	var keys []interface{}
	var keyExprs []ast.Expr
	var values []interface{}
	for _, op := range entries {
		k, ok := ctx.evaluateExpr(op.Key)
		if !ok {
			allOk = false
		}
		keys = append(keys, k)
		keyExprs = append(keyExprs, op.Key)

		v, ok := ctx.evaluateExpr(op.Value)
		if !ok {
			allOk = false
		}
		values = append(values, v)
	}

	if !allOk {
		return nil, false
	}

	evalObjectF := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		returnMap := map[string]interface{}{}
		allOk := true
		for i, arg := range args {
			if k, ok := arg.(string); ok {
				returnMap[k] = values[i]
			} else {
				ctx.error(keyExprs[i], fmt.Sprintf("object key must evaluate to a string, not %v", typeString(k)))
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
			if p, ok := p.(poisonMarker); ok {
				return p, true
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
	return pulumi.UnsafeUnknownOutput(nil)
}

// evaluatePropertyAccess evaluates interpolation expressions, `${foo.bar[baz]}`. The first item in
// the property access list is the head, and must be an identifier for a resource, config, or
// variable. The tail of property accessors are either: `.foo` string literal property names or
// `[42]` numeric literal property subscripts.
func (ctx *evalContext) evaluatePropertyAccess(expr ast.Expr, access *ast.PropertyAccess) (interface{}, bool) {
	resourceName := access.RootName()
	var receiver interface{}
	if res, ok := ctx.resources[resourceName]; ok {
		receiver = res
	} else if p, ok := ctx.config[resourceName]; ok {
		receiver = p
	} else if v, ok := ctx.variables[resourceName]; ok {
		receiver = v
	} else {
		return ctx.error(expr, fmt.Sprintf("resource or variable named %q could not be found", resourceName))
	}

	return ctx.evaluatePropertyAccessTail(expr, receiver, access.Accessors[1:])
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
					err := ctx.ctx.RegisterResource("_", "_", nil, res, pulumi.URN_(string(ref.URN)))
					if err != nil {
						ctx.error(expr, fmt.Sprintf("Failed to get resource %q: %v", ref.URN, err))
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

	var opts []pulumi.InvokeOption

	if t.CallOpts.Version != nil {
		opts = append(opts, pulumi.Version(t.CallOpts.Version.Value))
	}
	if t.CallOpts.PluginDownloadURL != nil {
		opts = append(opts, pulumi.PluginDownloadURL(t.CallOpts.PluginDownloadURL.Value))
	}
	if t.CallOpts.Parent != nil {
		parentOpt, ok := ctx.evaluateResourceValuedOption(t.CallOpts.Parent, "parent")
		if ok {
			if p, ok := parentOpt.(poisonMarker); ok {
				return p, true
			}
			opts = append(opts, pulumi.Parent(parentOpt.CustomResource()))
		} else {
			ctx.error(t.Return, fmt.Sprintf("Unable to evaluate options Parent field: %+v", t.CallOpts.Parent))
		}
	}
	if t.CallOpts.Provider != nil {
		providerOpt, ok := ctx.evaluateResourceValuedOption(t.CallOpts.Provider, "provider")
		if ok {
			if p, ok := providerOpt.(poisonMarker); ok {
				return p, true
			}
			provider := providerOpt.ProviderResource()
			if provider == nil {
				ctx.error(t.CallOpts.Provider, fmt.Sprintf("resource passed as Provider was not a provider resource '%s'", providerOpt))
			} else {
				opts = append(opts, pulumi.Provider(provider))
			}
		} else {
			ctx.error(t.Return, fmt.Sprintf("Unable to evaluate options Provider field: %+v", t.CallOpts.Provider))
		}
	}
	performInvoke := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		// At this point, we've got a function to invoke and some parameters! Invoke away.
		result := map[string]interface{}{}
		_, functionName, err := ResolveFunction(ctx.pkgLoader, t.Token.Value)
		if err != nil {
			return ctx.error(t, err.Error())
		}

		if err := ctx.ctx.Invoke(string(functionName), args[0], &result, opts...); err != nil {
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
	overallOk := true

	delim, ok := ctx.evaluateExpr(v.Delimiter)
	overallOk = overallOk && ok

	items, ok := ctx.evaluateExpr(v.Values)
	overallOk = overallOk && ok

	if !overallOk {
		return nil, false
	}

	join := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		overallOk := true

		delim := args[0]
		if delim == nil {
			delim = ""
		}
		delimStr, ok := delim.(string)
		overallOk = overallOk && ok
		if !ok {
			ctx.error(v.Delimiter, fmt.Sprintf("delimiter must be a string, not %v", typeString(args[0])))
		}

		parts, ok := args[1].([]interface{})
		overallOk = overallOk && ok
		if !ok {
			ctx.error(v.Values, fmt.Sprintf("the second argument to Fn::Join must be a list, found %v", typeString(args[1])))
		}

		if !overallOk {
			return nil, false
		}

		strs := make([]string, len(parts))
		for i, p := range parts {
			str, ok := p.(string)
			if !ok {
				ctx.error(v.Values, fmt.Sprintf("the second argument to Fn::Join must be a list of strings, found %v at index %v", typeString(p), i))
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

func (ctx *evalContext) evaluateBuiltinFromBase64(v *ast.FromBase64Expr) (interface{}, bool) {
	str, ok := ctx.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}
	fromBase64 := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		s, ok := args[0].(string)
		if !ok {
			return ctx.error(v.Value, fmt.Sprintf("expected argument to Fn::FromBase64 to be a string, got %v", typeString(args[0])))
		}
		b, err := b64.StdEncoding.DecodeString(s)
		if err != nil {
			return ctx.error(v.Value, fmt.Sprintf("Fn::FromBase64 unable to decode %v, error: %v", args[0], err))
		}
		decoded := string(b)
		if !utf8.ValidString(decoded) {
			return ctx.error(v.Value, "Fn::FromBase64 output is not a valid UTF-8 string")
		}
		return decoded, true
	})
	return fromBase64(str)
}

func (ctx *evalContext) evaluateBuiltinToBase64(v *ast.ToBase64Expr) (interface{}, bool) {
	str, ok := ctx.evaluateExpr(v.Value)
	if !ok {
		return nil, false
	}
	toBase64 := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		s, ok := args[0].(string)
		if !ok {
			return ctx.error(v.Value, fmt.Sprintf("expected argument to Fn::ToBase64 to be a string, got %v", typeString(args[0])))
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

func (ctx *evalContext) evaluateBuiltinSecret(s *ast.SecretExpr) (interface{}, bool) {
	e, ok := ctx.evaluateExpr(s.Value)
	if !ok {
		return nil, false
	}
	return pulumi.ToSecret(e), true
}

func (ctx *evalContext) evaluateBuiltinReadFile(s *ast.ReadFileExpr) (interface{}, bool) {
	e, ok := ctx.evaluateExpr(s.Path)
	if !ok {
		return nil, false
	}

	_, isConstant := s.Path.(*ast.StringExpr)

	readFileF := ctx.lift(func(args ...interface{}) (interface{}, bool) {
		path, ok := args[0].(string)
		if !ok {
			return ctx.error(s.Path, fmt.Sprintf("Argument to Fn::ReadFile must be a string, got %v", reflect.TypeOf(args[0])))
		}

		path = filepath.Clean(path)
		isAbsolute := filepath.IsAbs(path)
		path, err := filepath.EvalSymlinks(path) // Evaluate symlinks to ensure we don't escape the current project dir
		if err != nil {
			ctx.error(s.Path, fmt.Sprintf("Error reading file at path %v: %v", path, err))
		}
		path, err = filepath.Abs(path) // Compute the absolute path to use a prefix to check if we're relative
		if err != nil {
			ctx.error(s.Path, fmt.Sprintf("Error reading file at path %v: %v", path, err))
		}
		isSubdirectory := false
		relPath, err := filepath.Rel(ctx.cwd, path)
		if err != nil {
			ctx.error(s.Path, fmt.Sprintf("Error reading file at path %v: %v", path, err))
		}

		if !strings.HasPrefix(relPath, "../") {
			isSubdirectory = true
		}

		// Security, defense in depth: prevent path traversal exploits from leaking any information
		// (secrets, tokens, ...) from outside the project directory.
		//
		// Allow subdirectory paths, these are valid constructions of the form:
		//
		//  * "./README.md"
		//  * "${pulumi.cwd}/README.md"
		//  * ... etc
		//
		// Allow constant paths that are absolute, therefore reviewable:
		//
		//  * /etc/lsb-release
		//  * /usr/share/nginx/html
		//  * /var/run/secrets/kubernetes.io/serviceaccount/token
		//
		// Forbidding parent directory path traversals (Path Traversal vulnerability):
		//
		//  * ../../etc/shadow
		//  * ../../.ssh/id_rsa.pub
		if isSubdirectory || (isConstant && isAbsolute) {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				ctx.error(s.Path, fmt.Sprintf("Error reading file at path %v: %v", path, err))
			}
			return string(data), true
		}

		return ctx.error(s.Path, fmt.Sprintf("Argument to Fn::ReadFile must be a constant or contained in the project dir"))
	})

	return readFileF(e)
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
