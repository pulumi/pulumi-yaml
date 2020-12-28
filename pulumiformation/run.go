package pulumiformation

import (
	"encoding/json"
	"io/ioutil"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
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

	// Read in the template file (for now, hard coded as Main.*).
	if b, err := ioutil.ReadFile(MainTemplate + ".json"); err == nil {
		if err = json.Unmarshal(b, &t); err != nil {
			return t, errors.Wrapf(err, "decoding template %s.json", MainTemplate)
		}
	} else if b, err := ioutil.ReadFile(MainTemplate + ".yaml"); err == nil {
		if err = yaml.Unmarshal(b, &TagProcessor{&t}); err != nil {
			return t, errors.Wrapf(err, "decoding template %s.yaml", MainTemplate)
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
	params    map[string]interface{}
	resources map[string]lateboundResource
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
		params:    make(map[string]interface{}),
		resources: make(map[string]lateboundResource),
	}
}

func (r *runner) Evaluate() error {
	// Evaluating the template takes multiple passes. Much of the template is static, like Metadata and Mappings,
	// and aren't interesting other than that they can be used below. Evaluation consists of these steps:
	//
	//     1) Parameters: populate parameters using config, verifying the values substituting
	//        default values as necessary. These will then be available during execution.
	///
	// The Parameters must be populated before proceeding, as they can be used during subsequent evaluation.

	// TODO

	//     2) Conditions: prepare the conditions which will produce boolean conditions we can use during execution.

	// TODO

	// Next comes the important bit:
	//
	//     3) Resources: evaluate all resources and their properties, registering each one with
	//        the Pulumi engine. Because properties may depend upon one another using features like
	//        Fn::GetAtt, order of evaluation matters. We prefer to execute resources in the order
	//        defined, but we generally need to evaluate resources in topologically sorted order.
	//        The order is constrained only by these implicit dependencies formed by referencing
	//        resource properties as inputs as well as the explicit dependencies specified by DependsOn.
	if err := r.registerResources(); err != nil {
		return err
	}

	// Finally:
	//
	//     4) Outputs: after all resources have been registered, we can compute the exported stack
	//        outputs by evaluating this section. These are simply key/value pairs.
	if err := r.registerOutputs(); err != nil {
		return err
	}

	// Note that we don't currently support Transforms or Macros. This might be interesting eventually, but
	// currently depends on a fair bit of the CloudFormation server-side machinery to work.

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
		err := r.ctx.RegisterResource(v.Type, k, untypedArgs(props), res, opts...)
		if err != nil {
			return errors.Wrapf(err, "registering resource %s", k)
		}
		r.resources[k] = state
	}

	return nil
}

func (r *runner) registerOutputs() error {
	for k, v := range r.t.Outputs {
		out, err := r.evaluateUntypedExpression(v.Value)
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
	default:
		panic("fatal: invalid expr type")
	}
}

// evaluateBuiltinRef evaluates a "Ref" builtin. This map entry has a single value, which represents
// the resource name whose ID will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinRef(v *Ref) (interface{}, error) {
	res, ok := r.resources[v.ResourceName]
	if !ok {
		return nil, errors.Errorf("resource Ref named %s could not be found", v.ResourceName)
	}
	return res.CustomResource().ID().ToStringOutput(), nil
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
		index, err := massageToInt(indexV)
		if err != nil {
			return nil, err
		}
		elems := args[1:]
		return elems[index], nil
	})
	return out, nil
}

var substitionRegexp = regexp.MustCompile(`\$\{([^\}]*)\}`)

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

	// Find all replacement expressions in the string, and construct the array of
	// parts, which may be strings or Outputs of evalauted expressions.
	matches := substitionRegexp.FindAllStringSubmatchIndex(v.String, -1)
	i := 0
	var parts []interface{}
	for _, match := range matches {
		parts = append(parts, v.String[i:match[0]])
		i = match[1]
		expr := v.String[match[2]:match[3]]
		v, err := r.evaluateBuiltinSubTemplateExpression(expr, substitutions)
		if err != nil {
			return nil, err
		}
		parts = append(parts, v)
	}
	parts = append(parts, v.String[i:])

	// Lift the concatenation of the parts into a StringOutput and return it.
	return joinStringOutputs(parts), nil
}

func (r *runner) evaluateBuiltinSubTemplateExpression(expr string, subs map[string]interface{}) (interface{}, error) {
	// If it's an index expression 'a.b', then treat as an `Fn::GetAtt`
	if parts := strings.Split(expr, "."); len(parts) > 1 {
		if len(parts) > 2 {
			return nil, errors.Errorf("expected expression '%s' in Fn::Sub to have at most one '.' property access", expr)
		}
		return r.evaluateBuiltinGetAtt(&GetAtt{
			ResourceName: parts[0],
			PropertyName: parts[1],
		})
	}
	// Else, if it's a string that's in the substitutions map, evaluate that expression in the substitution map
	if sub, ok := subs[expr]; ok {
		return sub, nil
	}
	// Else, treat as a `Ref`
	return r.evaluateBuiltinRef(&Ref{
		ResourceName: expr,
	})
}

func joinStringOutputs(parts []interface{}) pulumi.StringOutput {
	return pulumi.All(parts...).ApplyString(func(arr []interface{}) (string, error) {
		s := ""
		for _, x := range arr {
			xs, ok := x.(string)
			if !ok {
				return "", errors.Errorf("expected expression in Fn::Join or Fn::Sub to produce a string, got %v", reflect.TypeOf(x))
			}
			s += xs
		}
		return s, nil
	})
}

// massageToInt defines an implicit conversion from raw values to
// int for use in Fn::Select.
func massageToInt(v interface{}) (int, error) {
	switch t := v.(type) {
	case int:
		return t, nil
	case int32:
		return int(t), nil
	case int64:
		return int(t), nil
	case uint64:
		return int(t), nil
	case float32:
		return int(t), nil
	case float64:
		return int(t), nil
	case bool:
		if t {
			return 1, nil
		}
		return 0, nil
	case string:
		return strconv.Atoi(t)
	default:
		return 0, errors.Errorf("expected type that can be converted to int, got %v", reflect.TypeOf(v))
	}
}

// untypedArgs is an untyped interface for a bag of properties.
type untypedArgs map[string]interface{}

func (untypedArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]interface{})(nil)).Elem()
}
