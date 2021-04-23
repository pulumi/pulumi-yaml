package pulumiyaml

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi/config"
	"github.com/spf13/cast"
	"gopkg.in/yaml.v3"
)

// MainTemplate is the assumed name of the JSON template file.
// TODO: would be nice to permit multiple files, but we'd need to know which is "main", and there's
//     no notion of "import" so we'd need to be a bit more clever. Might be nice to mimic e.g. Kustomize.
//     One idea is to hijack Pulumi.yaml's "main" directive and then just globally toposort the rest.
const MainTemplate = "Main"

// Load a template from the current working directory
func Load() (Template, error) {
	t := Template{}

	// Read in the template file - search first for Main.json, then Main.yaml, then Pulumi.yaml.
	// The last of these will actually read the proram from the same Pulumi.yaml project file used by
	// Pulumi CLI, which now plays double duty, and allows a Pulumi deployment that uses a single file.
	if b, err := ioutil.ReadFile(MainTemplate + ".json"); err == nil {
		if err = json.Unmarshal(b, &t); err != nil {
			return t, errors.Wrapf(err, "decoding template %s.json", MainTemplate)
		}
	} else if b, err := ioutil.ReadFile(MainTemplate + ".yaml"); err == nil {
		if err = yaml.Unmarshal(b, &TagProcessor{&t}); err != nil {
			return t, errors.Wrapf(err, "decoding template %s.yaml", MainTemplate)
		}
	} else if b, err := ioutil.ReadFile("Pulumi.yaml"); err == nil {
		if err = yaml.Unmarshal(b, &TagProcessor{&t}); err != nil {
			return t, errors.Wrapf(err, "decoding template Pulumi.yaml")
		}
	} else {
		return t, errors.Wrapf(err, "reading template %s", MainTemplate)
	}

	return t, nil
}

// Run the JSON evaluator against a template using the given request/settings.
func Run(ctx *pulumi.Context) error {
	t, err := Load()
	if err != nil {
		return err
	}

	// Now "evaluate" the template.
	return newRunner(ctx, t).Evaluate()
}

type runner struct {
	ctx       *pulumi.Context
	t         Template
	config    map[string]interface{}
	resources map[string]lateboundResource
	stackRefs map[string]*pulumi.StackReference
}

// lateboundResource is an interface shared by lateboundCustomResourceState and
// lateboundProviderResourceState so that both normal and provider resources can be
// created and managed as part of a deployment.
type lateboundResource interface {
	GetOutput(k string) pulumi.Output
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

// GetOutput returns the named output of the resource.
func (st *lateboundCustomResourceState) GetOutput(k string) pulumi.Output {
	return st.Outputs.ApplyT(func(outputs map[string]interface{}) (interface{}, error) {
		out, ok := outputs[k]
		if !ok {
			return nil, errors.Errorf("no output '%s' on resource '%s'", k, st.name)
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

// GetOutput returns the named output of the resource.
func (st *lateboundProviderResourceState) GetOutput(k string) pulumi.Output {
	return st.Outputs.ApplyT(func(outputs map[string]interface{}) (interface{}, error) {
		out, ok := outputs[k]
		if !ok {
			return nil, errors.Errorf("no output '%s' on resource '%s'", k, st.name)
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

func newRunner(ctx *pulumi.Context, t Template) *runner {
	return &runner{
		ctx:       ctx,
		t:         t,
		config:    make(map[string]interface{}),
		resources: make(map[string]lateboundResource),
		stackRefs: make(map[string]*pulumi.StackReference),
	}
}

func (r *runner) Evaluate() error {
	if err := r.registerConfig(); err != nil {
		return err
	}
	// TODO: Conditions
	if err := r.registerResources(); err != nil {
		return err
	}
	if err := r.registerOutputs(); err != nil {
		return err
	}
	return nil
}

func (r *runner) registerConfig() error {
	for k, c := range r.t.Configuration {
		var v interface{}
		var err error
		switch c.Type {
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
						return err
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
			v = c.Default
		}
		// TODO: Validate AllowedPattern, AllowedValues, MaxValue, MaxLength, MinValue, MinLength
		if c.Secret != nil && *c.Secret {
			v = pulumi.ToSecret(v)
		}
		r.config[k] = v
	}
	return nil
}

func (r *runner) registerResources() error {
	// Topologically sort the resources based on implicit and explicit dependencies
	resnames, err := topologicallySortedResources(&r.t)
	if err != nil {
		return err
	}
	for _, k := range resnames {
		v := r.t.Resources[k]
		if _, has := r.resources[k]; has {
			return errors.Errorf("unexpected duplicate resource name '%s'", k)
		}
		if v == nil {
			// TODO:Non-resource names can end up in topologicallySortedResources, in cases where
			// they are config settings (or in the future, variables).  This should be handled at
			// the right layer instead.
			continue
		}

		// Read the properties and then evaluate them in case there are expressions contained inside.
		props := make(map[string]interface{})
		for k, v := range v.Properties {
			vv, err := r.evaluateUntypedExpression(v)
			if err != nil {
				return err
			}
			props[k] = vv
		}

		var opts []pulumi.ResourceOption
		if v.AdditionalSecretOutputs != nil {
			opts = append(opts, pulumi.AdditionalSecretOutputs(v.AdditionalSecretOutputs))
		}
		if v.Aliases != nil {
			var aliases []pulumi.Alias
			for _, s := range v.Aliases {
				alias := pulumi.Alias{
					URN: pulumi.URN(s),
				}
				aliases = append(aliases, alias)
			}
			opts = append(opts, pulumi.Aliases(aliases))
		}
		if v.CustomTimeouts != nil {
			opts = append(opts, pulumi.Timeouts(&pulumi.CustomTimeouts{
				Create: v.CustomTimeouts.Create,
				Delete: v.CustomTimeouts.Delete,
				Update: v.CustomTimeouts.Update,
			}))
		}
		if v.DeleteBeforeReplace {
			opts = append(opts, pulumi.DeleteBeforeReplace(v.DeleteBeforeReplace))
		}
		if v.DependsOn != nil {
			var dependsOn []pulumi.Resource
			for _, s := range v.DependsOn {
				dependsOn = append(dependsOn, r.resources[s].CustomResource())
			}
			opts = append(opts, pulumi.DependsOn(dependsOn))
		}
		if v.IgnoreChanges != nil {
			opts = append(opts, pulumi.IgnoreChanges(v.IgnoreChanges))
		}
		if v.Parent != "" {
			opts = append(opts, pulumi.Parent(r.resources[v.Parent].CustomResource()))
		}
		if v.Protect {
			opts = append(opts, pulumi.Protect(v.Protect))
		}
		if v.Provider != "" {
			provider := r.resources[v.Provider].ProviderResource()
			if provider == nil {
				return errors.Errorf("resource passed as Provider was not a provider resource '%s'", v.Provider)
			}
			opts = append(opts, pulumi.Provider(provider))
		}
		if v.Version != "" {
			opts = append(opts, pulumi.Version(v.Version))
		}

		// Create either a latebound custom resource or latebound provider resource depending on
		// whether the type token indicates a special provider type.
		var state lateboundResource
		var res pulumi.Resource
		if strings.HasPrefix(v.Type, "pulumi:providers:") {
			r := lateboundProviderResourceState{name: k}
			state = &r
			res = &r
		} else {
			r := lateboundCustomResourceState{name: k}
			state = &r
			res = &r
		}

		// Now register the resulting resource with the engine.
		var err error
		if v.Component {
			err = r.ctx.RegisterRemoteComponentResource(v.Type, k, untypedArgs(props), res, opts...)
		} else {
			err = r.ctx.RegisterResource(v.Type, k, untypedArgs(props), res, opts...)
		}
		if err != nil {
			return errors.Wrapf(err, "registering resource %s", k)
		}
		r.resources[k] = state
	}

	return nil
}

func (r *runner) registerOutputs() error {
	for k, v := range r.t.Outputs {
		out, err := r.evaluateUntypedExpression(v)
		if err != nil {
			return err
		}
		r.ctx.Export(k, pulumi.Any(out))
	}
	return nil
}

// evaluateUntypedExpression takes an object structure that is a mixture of JSON primitives and builtin functions
// and turns it into an untyped bag of values that are suitable to pass as inputs to the Pulumi Go SDK.
func (r *runner) evaluateUntypedExpression(v interface{}) (interface{}, error) {
	e, err := Parse(v)
	if err != nil {
		return nil, err
	}
	return r.evaluateExpr(e)
}

func (r *runner) evaluateExpr(e Expr) (interface{}, error) {
	switch t := e.(type) {
	case *Value:
		return t.Val, nil
	case *Array:
		var xs []interface{}
		for _, x := range t.Elems {
			vv, err := r.evaluateExpr(x)
			if err != nil {
				return nil, err
			}
			xs = append(xs, vv)
		}
		return xs, nil
	case *Object:
		m := make(map[string]interface{})
		for k, e := range t.Elems {
			vv, err := r.evaluateExpr(e)
			if err != nil {
				return nil, err
			}
			m[k] = vv
		}
		return m, nil
	case *Ref:
		return r.evaluateBuiltinRef(t)
	case *GetAtt:
		return r.evaluateBuiltinGetAtt(t)
	case *Invoke:
		return r.evaluateBuiltinInvoke(t)
	case *Join:
		return r.evaluateBuiltinJoin(t)
	case *Sub:
		return r.evaluateBuiltinSub(t)
	case *Select:
		return r.evaluateBuiltinSelect(t)
	case *Asset:
		return r.evaluateBuiltinAsset(t)
	case *StackReference:
		return r.evaluateBuiltinStackReference(t)
	default:
		panic(fmt.Sprintf("fatal: invalid expr type %v", reflect.TypeOf(e)))
	}
}

// evaluateBuiltinRef evaluates a "Ref" builtin. This map entry has a single value, which represents
// the resource name whose ID will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinRef(v *Ref) (interface{}, error) {
	res, ok := r.resources[v.ResourceName]
	if ok {
		return res.CustomResource().ID().ToStringOutput(), nil
	}
	p, ok := r.config[v.ResourceName]
	if ok {
		return p, nil
	}
	return nil, errors.Errorf("resource Ref named %s could not be found", v.ResourceName)
}

// evaluateBuiltinGetAtt evaluates a "GetAtt" builtin. This map entry has a single two-valued array,
// the first value being the resource name, and the second being the property to read, and whose
// value will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinGetAtt(v *GetAtt) (interface{}, error) {
	// Look up the resource and ensure it exists.
	res, ok := r.resources[v.ResourceName]
	if !ok {
		return nil, errors.Errorf("resource %s named by Fn::GetAtt could not be found", v.ResourceName)
	}

	// Get the requested property's output value from the resource state
	return res.GetOutput(v.PropertyName), nil
}

// evaluateBuiltinInvoke evaluates the "Invoke" builtin, which enables templates to invoke arbitrary
// data source functions, to fetch information like the current availability zone, lookup AMIs, etc.
func (r *runner) evaluateBuiltinInvoke(t *Invoke) (interface{}, error) {
	argVals := make(map[string]interface{})
	// The arguments may very well have expressions within them, so evaluate those now.
	for k, v := range t.Args.Elems {
		nv, err := r.evaluateExpr(v)
		if err != nil {
			return nil, err
		}
		argVals[k] = nv
	}

	// At this point, we've got a function to invoke and some parameters! Invoke away.
	result := make(map[string]interface{})
	if err := r.ctx.Invoke(t.Token, argVals, &result); err != nil {
		return nil, err
	}
	retv, ok := result[t.Return]
	if !ok {
		return nil, errors.Errorf(
			"Fn::Invoke of %s did not contain a property '%s' in the returned value", t.Token, t.Return)
	}
	return retv, nil
}

func (r *runner) evaluateBuiltinJoin(v *Join) (interface{}, error) {
	delim, err := r.evaluateExpr(v.Delimiter)
	if err != nil {
		return nil, err
	}
	var parts []interface{}
	for i, e := range v.Values.Elems {
		if i != 0 {
			parts = append(parts, delim)
		}
		part, err := r.evaluateExpr(e)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	return joinStringOutputs(parts), nil
}

func (r *runner) evaluateBuiltinSelect(v *Select) (interface{}, error) {
	index, err := r.evaluateExpr(v.Index)
	if err != nil {
		return nil, err
	}
	var elems []interface{}
	for _, e := range v.Values.Elems {
		ev, err := r.evaluateExpr(e)
		if err != nil {
			return nil, err
		}
		elems = append(elems, ev)
	}
	args := append([]interface{}{index}, elems...)
	out := pulumi.All(args...).ApplyT(func(args []interface{}) (interface{}, error) {
		indexV := args[0]
		index, err := cast.ToIntE(indexV)
		if err != nil {
			return nil, err
		}
		elems := args[1:]
		return elems[index], nil
	})
	return out, nil
}

func (r *runner) evaluateBuiltinSub(v *Sub) (interface{}, error) {
	// Evaluate all the substition mapping expressions.
	substitutions := make(map[string]interface{})
	if v.Substitutions != nil {
		for k, sub := range v.Substitutions.Elems {
			sub, err := r.evaluateExpr(sub)
			if err != nil {
				return "", err
			}
			substitutions[k] = sub
		}
	}

	var parts []interface{}

	var i int
	var expr Expr
	for i, expr = range v.ExpressionParts {
		parts = append(parts, v.StringParts[i])
		// TODO: We are (no longer) handling the subsitutions as part of Fn::Sub.  We will need to introduce
		// a notion of local scopes so we can inject these in scope for the Ref to lookup.
		v, err := r.evaluateExpr(expr)
		if err != nil {
			return nil, err
		}
		parts = append(parts, v)
	}
	parts = append(parts, v.StringParts[i+1])

	// Lift the concatenation of the parts into a StringOutput and return it.
	return joinStringOutputs(parts), nil
}

func (r *runner) evaluateBuiltinAsset(v *Asset) (interface{}, error) {
	switch v.Kind {
	case FileAsset:
		return pulumi.NewFileAsset(v.Path), nil
	case StringAsset:
		return pulumi.NewStringAsset(v.Path), nil
	case RemoteAsset:
		return pulumi.NewRemoteAsset(v.Path), nil
	default:
		return nil, errors.Errorf("unexpected Asset kind '%s'", v.Kind)
	}
}

func (r *runner) evaluateBuiltinStackReference(v *StackReference) (interface{}, error) {
	stackRef, ok := r.stackRefs[v.StackName]
	if !ok {
		var err error
		stackRef, err = pulumi.NewStackReference(r.ctx, v.StackName, &pulumi.StackReferenceArgs{})
		if err != nil {
			return nil, err
		}
		r.stackRefs[v.StackName] = stackRef
	}

	property, err := r.evaluateExpr(v.PropertyName)
	if err != nil {
		return nil, err
	}

	propertyStringOutput := pulumi.ToOutput(property).ApplyT(func(v interface{}) (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", errors.Errorf("expected property name argument to Fn::StackReference to be a string, got %v", reflect.TypeOf(v))
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
				return "", errors.Errorf("expected expression in Fn::Join or Fn::Sub to produce a string, got %v", reflect.TypeOf(x))
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
