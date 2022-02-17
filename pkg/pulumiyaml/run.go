// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
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
		return err, true
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

	// Now "evaluate" the template.
	return RunTemplate(ctx, t)
}

// RunTemplate runs the evaluator against a template using the given request/settings.
func RunTemplate(ctx *pulumi.Context, t *ast.TemplateDecl) error {
	diags := newRunner(ctx, t).Evaluate()

	if diags.HasErrors() {
		return diags
	}
	return nil
}

type runner struct {
	ctx       *pulumi.Context
	t         *ast.TemplateDecl
	config    map[string]interface{}
	resources map[string]lateboundResource
	stackRefs map[string]*pulumi.StackReference
}

// lateboundResource is an interface shared by lateboundCustomResourceState and
// lateboundProviderResourceState so that both normal and provider resources can be
// created and managed as part of a deployment.
type lateboundResource interface {
	GetOutput(k string) pulumi.Output
	GetOutputs() pulumi.Output
	CustomResource() *pulumi.CustomResourceState
	ProviderResource() *pulumi.ProviderResourceState
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

func newRunner(ctx *pulumi.Context, t *ast.TemplateDecl) *runner {
	return &runner{
		ctx:       ctx,
		t:         t,
		config:    make(map[string]interface{}),
		resources: make(map[string]lateboundResource),
		stackRefs: make(map[string]*pulumi.StackReference),
	}
}

func (r *runner) Evaluate() syntax.Diagnostics {
	var diags syntax.Diagnostics

	cdiags := r.registerConfig()
	diags.Extend(cdiags...)
	if cdiags.HasErrors() {
		return diags
	}

	// TODO: Conditions

	rdiags := r.registerResources()
	diags.Extend(rdiags...)
	if rdiags.HasErrors() {
		return diags
	}

	odiags := r.registerOutputs()
	diags.Extend(odiags...)
	return diags
}

func (r *runner) registerConfig() syntax.Diagnostics {
	if r.t.Configuration == nil {
		return nil
	}

	var diags syntax.Diagnostics
	for _, kvp := range r.t.Configuration.Entries {
		k, c := kvp.Key.Value, kvp.Value

		var defaultValue interface{}
		if c.Default != nil {
			d, ddiags := r.evaluateExpr(c.Default)
			diags.Extend(ddiags...)
			if ddiags.HasErrors() {
				continue
			}
			defaultValue = d
		}

		var v interface{}
		var err error
		switch c.Type.Value {
		case "String":
			v, err = config.Try(r.ctx, k)
		case "Number":
			v, err = config.TryFloat64(r.ctx, k)
		case "List<Number>":
			v, err = config.Try(r.ctx, k)
			if err == nil {
				var arr []float64
				for _, nstr := range strings.Split(v.(string), ",") {
					f, err := cast.ToFloat64E(nstr)
					if err != nil {
						diags.Extend(ast.ExprError(kvp.Key, err.Error(), ""))
						continue
					}
					arr = append(arr, f)
				}
				v = arr
			}
		case "CommaDelimitedList":
			v, err = config.Try(r.ctx, k)
			if err == nil {
				v = strings.Split(v.(string), ",")
			}
		}
		if err != nil {
			v = defaultValue
		}
		// TODO: Validate AllowedPattern, AllowedValues, MaxValue, MaxLength, MinValue, MinLength
		if c.Secret != nil && c.Secret.Value {
			v = pulumi.ToSecret(v)
		}
		r.config[k] = v
	}
	return nil
}

func (r *runner) registerResources() syntax.Diagnostics {
	// Topologically sort the resources based on implicit and explicit dependencies
	resources, rdiags := topologicallySortedResources(r.t)
	if rdiags.HasErrors() {
		return rdiags
	}

	var diags syntax.Diagnostics
	for _, kvp := range resources {
		k, v := kvp.Key.Value, kvp.Value

		// Read the properties and then evaluate them in case there are expressions contained inside.
		props := make(map[string]interface{})
		if v.Properties != nil {
			for _, kvp := range v.Properties.Entries {
				vv, vdiags := r.evaluateExpr(kvp.Value)
				diags.Extend(vdiags...)
				if vdiags.HasErrors() {
					return diags
				}
				props[kvp.Key.Value] = vv
			}
		} else {
			// TODO: Make this a diagnostic warning?
			// diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("resource %v passed has an empty properties value", kvp.Key.Value), ""))
		}

		var opts []pulumi.ResourceOption
		if v.Options != nil {
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
				var dependsOn []pulumi.Resource
				for _, s := range v.Options.DependsOn.Elements {
					dependsOn = append(dependsOn, r.resources[s.Value].CustomResource())
				}
				opts = append(opts, pulumi.DependsOn(dependsOn))
			}
			if v.Options.IgnoreChanges != nil {
				opts = append(opts, pulumi.IgnoreChanges(listStrings(v.Options.IgnoreChanges)))
			}
			if v.Options.Parent != nil && v.Options.Parent.Value != "" {
				opts = append(opts, pulumi.Parent(r.resources[v.Options.Parent.Value].CustomResource()))
			}
			if v.Options.Protect != nil {
				opts = append(opts, pulumi.Protect(v.Options.Protect.Value))
			}
			if v.Options.Provider != nil && v.Options.Provider.Value != "" {
				provider := r.resources[v.Options.Provider.Value].ProviderResource()
				if provider == nil {
					diags.Extend(ast.ExprError(v.Options.Provider, fmt.Sprintf("resource passed as Provider was not a provider resource '%s'", v.Options.Provider.Value), ""))
					return diags
				}
				opts = append(opts, pulumi.Provider(provider))
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
		}

		// Create either a latebound custom resource or latebound provider resource depending on
		// whether the type token indicates a special provider type.
		var state lateboundResource
		var res pulumi.Resource
		if strings.HasPrefix(v.Type.Value, "pulumi:providers:") {
			r := lateboundProviderResourceState{name: k}
			state = &r
			res = &r
		} else {
			r := lateboundCustomResourceState{name: k}
			state = &r
			res = &r
		}

		// If the provided type token is `pkg:type`, expand it to `pkd:index:type` automatically.  We may
		// well want to handle this more fundamentally in Pulumi itself to avoid the need for `:index:`
		// ceremony quite generally.
		typ := v.Type.Value
		typParts := strings.Split(typ, ":")
		if len(typParts) < 2 || len(typParts) > 3 {
			diags.Extend(ast.ExprError(v.Type, fmt.Sprintf("invalid type token %q for resource %q", v.Type.Value, k), ""))
			return diags
		} else if len(typParts) == 2 {
			typ = fmt.Sprintf("%s:index:%s", typParts[0], typParts[1])
		}

		// Now register the resulting resource with the engine.
		var err error
		if v.Component != nil && v.Component.Value {
			err = r.ctx.RegisterRemoteComponentResource(typ, k, untypedArgs(props), res, opts...)
		} else {
			err = r.ctx.RegisterResource(typ, k, untypedArgs(props), res, opts...)
		}
		if err != nil {
			diags.Extend(ast.ExprError(kvp.Key, err.Error(), ""))
			return diags
		}
		r.resources[k] = state
	}

	if diags.HasErrors() {
		return diags
	}

	return nil
}

func (r *runner) registerOutputs() syntax.Diagnostics {
	if r.t.Outputs == nil {
		return nil
	}

	var diags syntax.Diagnostics
	for _, kvp := range r.t.Outputs.Entries {
		out, odiags := r.evaluateExpr(kvp.Value)
		diags.Extend(odiags...)
		if odiags.HasErrors() {
			return diags
		}
		r.ctx.Export(kvp.Key.Value, pulumi.Any(out))
	}
	return diags
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
func (r *runner) evaluateExpr(x ast.Expr) (interface{}, syntax.Diagnostics) {
	switch x := x.(type) {
	case *ast.NullExpr:
		return nil, nil
	case *ast.BooleanExpr:
		return x.Value, nil
	case *ast.NumberExpr:
		return x.Value, nil
	case *ast.StringExpr:
		return x.Value, nil
	case *ast.ListExpr:
		return r.evaluateList(x)
	case *ast.ObjectExpr:
		return r.evaluateObject(x, nil, map[string]interface{}{}, x.Entries)
	case *ast.InterpolateExpr:
		return r.evaluateInterpolate(x, nil)
	case *ast.SymbolExpr:
		return r.evaluatePropertyAccess(x, x.Property, nil)
	case *ast.RefExpr:
		return r.evaluateBuiltinRef(x)
	case *ast.GetAttExpr:
		return r.evaluateBuiltinGetAtt(x)
	case *ast.InvokeExpr:
		return r.evaluateBuiltinInvoke(x)
	case *ast.JoinExpr:
		return r.evaluateBuiltinJoin(x)
	case *ast.SubExpr:
		return r.evaluateBuiltinSub(x)
	case *ast.SelectExpr:
		return r.evaluateBuiltinSelect(x)
	case *ast.AssetExpr:
		return r.evaluateBuiltinAsset(x)
	case *ast.StackReferenceExpr:
		return r.evaluateBuiltinStackReference(x)
	default:
		panic(fmt.Sprintf("fatal: invalid expr type %v", reflect.TypeOf(x)))
	}
}

func (r *runner) evaluateList(x *ast.ListExpr) (interface{}, syntax.Diagnostics) {
	var diags syntax.Diagnostics
	xs := make([]interface{}, len(x.Elements))
	for i, e := range x.Elements {
		ev, ediags := r.evaluateExpr(e)
		diags.Extend(ediags...)
		if ediags.HasErrors() {
			return nil, diags
		}
		xs[i] = ev
	}
	return xs, diags
}

func (r *runner) evaluateObject(x *ast.ObjectExpr, diags syntax.Diagnostics, m map[string]interface{}, entries []ast.ObjectProperty) (interface{}, syntax.Diagnostics) {
	if len(entries) == 0 {
		return m, diags
	}

	kvp := entries[0]

	kv, kdiags := r.evaluateExpr(kvp.Key)
	diags.Extend(kdiags...)
	if kdiags.HasErrors() {
		return nil, diags
	}

	if o, ok := kv.(pulumi.Output); ok {
		return o.ApplyT(func(kv interface{}) (interface{}, error) {
			// TODO: this could leak warnings
			v, diags := r.continueObject(x, diags, m, kvp, kv, entries)
			if diags.HasErrors() {
				return nil, diags
			}
			return v, diags
		}), nil
	}

	return r.continueObject(x, diags, m, kvp, kv, entries)
}

func (r *runner) continueObject(x *ast.ObjectExpr, diags syntax.Diagnostics, m map[string]interface{}, kvp ast.ObjectProperty, kv interface{}, entries []ast.ObjectProperty) (interface{}, syntax.Diagnostics) {
	k, ok := kv.(string)
	if !ok {
		diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("object key must evaluate to a string, not %v", typeString(kv)), ""))
		return nil, diags
	}

	v, vdiags := r.evaluateExpr(kvp.Value)
	diags.Extend(vdiags...)
	if vdiags.HasErrors() {
		return nil, diags
	}

	m[k] = v
	return r.evaluateObject(x, diags, m, entries[1:])
}

func (r *runner) evaluateInterpolate(x *ast.InterpolateExpr, subs map[string]interface{}) (interface{}, syntax.Diagnostics) {
	return r.evaluateInterpolations(x, nil, &strings.Builder{}, x.Parts, subs)
}

func (r *runner) evaluateInterpolations(x *ast.InterpolateExpr, diags syntax.Diagnostics, b *strings.Builder, parts []ast.Interpolation, subs map[string]interface{}) (interface{}, syntax.Diagnostics) {
	for ; len(parts) > 0; parts = parts[1:] {
		i := parts[0]
		b.WriteString(i.Text)

		if i.Value != nil {
			p, pdiags := r.evaluatePropertyAccess(x, i.Value, subs)
			diags.Extend(pdiags...)
			if pdiags.HasErrors() {
				return nil, diags
			}

			if o, ok := p.(pulumi.Output); ok {
				return o.ApplyT(func(v interface{}) (interface{}, error) {
					fmt.Fprintf(b, "%v", v)
					// TODO: this could leak warnings
					v, diags := r.evaluateInterpolations(x, diags, b, parts[1:], subs)
					if diags.HasErrors() {
						return nil, diags
					}
					return v, nil
				}), nil
			}

			fmt.Fprintf(b, "%v", p)
		}
	}
	return b.String(), diags
}

func (r *runner) evaluatePropertyAccess(x ast.Expr, access *ast.PropertyAccess, subs map[string]interface{}) (interface{}, syntax.Diagnostics) {
	resourceName := access.Accessors[0].(*ast.PropertyName).Name

	var receiver interface{}
	if res, ok := r.resources[resourceName]; ok {
		if len(access.Accessors) == 1 {
			return res.CustomResource().ID().ToStringOutput(), nil
		}
		receiver = res.GetOutputs()
	} else if p, ok := r.config[resourceName]; ok {
		receiver = p
	} else if s, ok := subs[resourceName]; ok {
		receiver = s
	} else {
		return nil, syntax.Diagnostics{ast.ExprError(x, fmt.Sprintf("resource named %s could not be found", resourceName), "")}
	}

	v, err := r.evaluateAccess(receiver, access.Accessors[1:])
	if err != nil {
		return nil, syntax.Diagnostics{ast.ExprError(x, err.Error(), "")}
	}
	return v, nil
}

func (r *runner) evaluateAccess(receiver interface{}, accessors []ast.PropertyAccessor) (interface{}, error) {
	for ; len(accessors) > 0; accessors = accessors[1:] {
		switch x := receiver.(type) {
		case pulumi.Output:
			return x.ApplyT(func(v interface{}) (interface{}, error) {
				return r.evaluateAccess(v, accessors)
			}), nil
		case []interface{}:
			sub, ok := accessors[0].(*ast.PropertySubscript)
			if !ok {
				return nil, fmt.Errorf("cannot access a list element using a property name")
			}
			index, ok := sub.Index.(int)
			if !ok {
				return nil, fmt.Errorf("cannot access a list element using a property name")
			}
			if index < 0 || index >= len(x) {
				return nil, fmt.Errorf("list index %v out-of-bounds for list of length %v", index, len(x))
			}
			receiver = x[index]
		case map[string]interface{}:
			var k string
			switch a := accessors[0].(type) {
			case *ast.PropertyName:
				k = a.Name
			case *ast.PropertySubscript:
				s, ok := a.Index.(string)
				if !ok {
					return nil, fmt.Errorf("cannot access an object property using an integer index")
				}
				k = s
			}
			receiver = x[k]
		default:
			return nil, fmt.Errorf("receiver must be a list or object, not %v", typeString(receiver))
		}
	}
	return receiver, nil
}

// evaluateBuiltinRef evaluates a "Ref" builtin. This map entry has a single value, which represents
// the resource name whose ID will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinRef(v *ast.RefExpr) (interface{}, syntax.Diagnostics) {
	res, ok := r.resources[v.ResourceName.Value]
	if ok {
		return res.CustomResource().ID().ToStringOutput(), nil
	}
	p, ok := r.config[v.ResourceName.Value]
	if ok {
		return p, nil
	}
	return nil, syntax.Diagnostics{ast.ExprError(v, fmt.Sprintf("resource Ref named %s could not be found", v.ResourceName.Value), "")}
}

// evaluateBuiltinGetAtt evaluates a "GetAtt" builtin. This map entry has a single two-valued array,
// the first value being the resource name, and the second being the property to read, and whose
// value will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinGetAtt(v *ast.GetAttExpr) (interface{}, syntax.Diagnostics) {
	// Look up the resource and ensure it exists.
	res, ok := r.resources[v.ResourceName.Value]
	if !ok {
		return nil, syntax.Diagnostics{ast.ExprError(v, fmt.Sprintf("resource %s named by Fn::GetAtt could not be found", v.ResourceName.Value), "")}
	}

	// Get the requested property's output value from the resource state
	return res.GetOutput(v.PropertyName.Value), nil
}

// evaluateBuiltinInvoke evaluates the "Invoke" builtin, which enables templates to invoke arbitrary
// data source functions, to fetch information like the current availability zone, lookup AMIs, etc.
func (r *runner) evaluateBuiltinInvoke(t *ast.InvokeExpr) (interface{}, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	args, adiags := r.evaluateExpr(t.CallArgs)
	diags.Extend(adiags...)
	if adiags.HasErrors() {
		return nil, diags
	}

	performInvoke := func(args interface{}) (interface{}, syntax.Diagnostics) {
		// At this point, we've got a function to invoke and some parameters! Invoke away.
		result := map[string]interface{}{}
		if err := r.ctx.Invoke(t.Token.Value, args, &result); err != nil {
			diags.Extend(ast.ExprError(t, err.Error(), ""))
			return nil, diags
		}
		retv, ok := result[t.Return.Value]
		if !ok {
			diags.Extend(ast.ExprError(t.Return, fmt.Sprintf("Fn::Invoke of %s did not contain a property '%s' in the returned value", t.Token.Value, t.Return.Value), ""))
			return nil, diags
		}
		return retv, diags
	}

	// TODO[pulumi/pulumi-yaml#14]: Use dynamic Output-or-not information to decide whether the lift the invoke
	var deps []*ast.StringExpr
	getExpressionDependencies(&deps, t.CallArgs)

	if len(deps) > 0 {
		return pulumi.ToOutput(args).ApplyT(func(args interface{}) (interface{}, error) {
			result, diags := performInvoke(args)
			if diags.HasErrors() {
				// TODO: this could leak warnings
				// Note for reviewer: will need to plumb through a context to providing non-error diagnostics outside of ApplyT
				return nil, diags
			}
			return result, nil
		}), nil
	}

	return performInvoke(args)
}

func (r *runner) evaluateBuiltinJoin(v *ast.JoinExpr) (interface{}, syntax.Diagnostics) {
	var diags syntax.Diagnostics
	delim, ddiags := r.evaluateExpr(v.Delimiter)
	diags.Extend(ddiags...)
	if ddiags.HasErrors() {
		return nil, ddiags
	}
	var parts []interface{}
	for i, e := range v.Values.Elements {
		if i != 0 {
			parts = append(parts, delim)
		}
		part, pdiags := r.evaluateExpr(e)
		diags.Extend(pdiags...)
		if pdiags.HasErrors() {
			return nil, diags
		}
		parts = append(parts, part)
	}
	return joinStringOutputs(parts), diags
}

func (r *runner) evaluateBuiltinSelect(v *ast.SelectExpr) (interface{}, syntax.Diagnostics) {
	var diags syntax.Diagnostics
	index, idiags := r.evaluateExpr(v.Index)
	diags.Extend(idiags...)
	if idiags.HasErrors() {
		return nil, idiags
	}
	var elems []interface{}
	for _, e := range v.Values.Elements {
		ev, ediags := r.evaluateExpr(e)
		diags.Extend(ediags...)
		if ediags.HasErrors() {
			return nil, diags
		}
		elems = append(elems, ev)
	}
	args := append([]interface{}{index}, elems...)
	out := pulumi.All(args...).ApplyT(func(args []interface{}) (interface{}, error) {
		indexV := args[0]
		index, err := cast.ToIntE(indexV)
		if err != nil {
			diags.Extend(ast.ExprError(v.Index, err.Error(), ""))
			return nil, diags
		}
		elems := args[1:]
		// TODO: this could leak warnings
		if diags.HasErrors() {
			return nil, diags
		}
		return elems[index], nil
	})
	return out, nil
}

func (r *runner) evaluateBuiltinSub(v *ast.SubExpr) (interface{}, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	// Evaluate all the substition mapping expressions.
	substitutions := make(map[string]interface{})
	if v.Substitutions != nil {
		for _, kvp := range v.Substitutions.Entries {
			k := kvp.Key.(*ast.StringExpr).Value

			v, vdiags := r.evaluateExpr(kvp.Value)
			diags.Extend(vdiags...)
			if vdiags.HasErrors() {
				return nil, diags
			}
			substitutions[k] = v
		}
	}
	return r.evaluateInterpolate(v.Interpolate, substitutions)
}

func (r *runner) evaluateBuiltinAsset(v *ast.AssetExpr) (interface{}, syntax.Diagnostics) {
	switch v.Kind.Value {
	case "File":
		return pulumi.NewFileAsset(v.Path.Value), nil
	case "String":
		return pulumi.NewStringAsset(v.Path.Value), nil
	case "Remote":
		return pulumi.NewRemoteAsset(v.Path.Value), nil
	case "FileArchive":
		return pulumi.NewFileArchive(v.Path.Value), nil
	case "RemoteArchive":
		return pulumi.NewRemoteArchive(v.Path.Value), nil
	case "AssetArchive":
		// TODO[pulumi/pulumi-yaml#53]: Implement Fn::Archive or support all variants as args to Fn::Asset
		panic(fmt.Errorf("%s unimplemented", v.Kind.Value))
	default:
		panic(fmt.Errorf("unexpected Asset kind '%s'", v.Kind.Value))
	}
}

func (r *runner) evaluateBuiltinStackReference(v *ast.StackReferenceExpr) (interface{}, syntax.Diagnostics) {
	stackRef, ok := r.stackRefs[v.StackName.Value]
	if !ok {
		var err error
		stackRef, err = pulumi.NewStackReference(r.ctx, v.StackName.Value, &pulumi.StackReferenceArgs{})
		if err != nil {
			return nil, syntax.Diagnostics{ast.ExprError(v.StackName, err.Error(), "")}
		}
		r.stackRefs[v.StackName.Value] = stackRef
	}

	var diags syntax.Diagnostics

	property, pdiags := r.evaluateExpr(v.PropertyName)
	diags.Extend(pdiags...)
	if pdiags.HasErrors() {
		return nil, diags
	}

	propertyStringOutput := pulumi.ToOutput(property).ApplyT(func(n interface{}) (string, error) {
		s, ok := n.(string)
		if !ok {
			diags.Extend(ast.ExprError(
				v.PropertyName,
				fmt.Sprintf("expected property name argument to Fn::StackReference to be a string, got %v", typeString(n)), ""),
			)
		}
		// TODO: this could leak warnings
		if diags.HasErrors() {
			return "", diags
		}
		return s, nil
	}).(pulumi.StringOutput)

	return stackRef.GetOutput(propertyStringOutput), nil
}

func joinStringOutputs(parts []interface{}) pulumi.StringOutput {
	return pulumi.All(parts...).ApplyT(func(arr []interface{}) (string, error) {
		s := ""
		for _, x := range arr {
			xs, ok := x.(string)
			if !ok {
				return "", fmt.Errorf("expected expression in Fn::Join or Fn::Sub to produce a string, got %v", reflect.TypeOf(x))
			}
			s += xs
		}
		return s, nil
	}).(pulumi.StringOutput)
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
