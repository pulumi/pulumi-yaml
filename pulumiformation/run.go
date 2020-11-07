package pulumiformation

import (
	"encoding/json"
	"io/ioutil"
	"reflect"

	"github.com/pkg/errors"

	"github.com/joeduffy/pulumiformation/pulumiformation/pulumi"
)

// MainTemplate is the assumed name of the JSON template file.
// TODO: would be nice to permit multiple files, but we'd need to know which is "main", and there's
//     no notion of "import" so we'd need to be a bit more clever. Might be nice to mimic e.g. Kustomize.
//     One idea is to hijack Pulumi.yaml's "main" directive and then just globally toposort the rest.
const MainTemplate = "Main.json"

// Run runs the JSON evaluator against a template using the given request/settings.
func Run(ctx *pulumi.Context) error {
	// Read in the template file (for now, hard coded as Main.*).
	b, err := ioutil.ReadFile(MainTemplate)
	if err != nil {
		return errors.Wrapf(err, "reading template %s", MainTemplate)
	}

	// Now decode the file using CloudFormation rules.
	// TODO: eventually this won't work because we'll need to customize (e.g., with !Invoke). Hopefully
	//     we can just fork rather than needing to recreate the CloudFormation oddities.
	t := Template{}
	if err = json.Unmarshal(b, &t); err != nil {
		return errors.Wrapf(err, "decoding template %s", MainTemplate)
	}

	// Now "evaluate" the template.
	return newRunner(ctx, t).Evaluate()
}

type runner struct {
	ctx       *pulumi.Context
	t         Template
	params    map[string]interface{}
	resources map[string]*registeredResource
}

type registeredResource struct {
	State   *pulumi.CustomResourceState
	Pending *pulumi.PendingResourceState
}

func newRunner(ctx *pulumi.Context, t Template) *runner {
	return &runner{
		ctx:       ctx,
		t:         t,
		params:    make(map[string]interface{}),
		resources: make(map[string]*registeredResource),
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
	// TODO: toposort.
	for k, v := range r.t.Resources {
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

		// Now register the resulting resource with the engine.
		var state pulumi.CustomResourceState
		pending, err := r.ctx.RegisterResource(v.Type, k, untypedArgs(props), &state)
		if err != nil {
			return errors.Wrapf(err, "registering resource %s/%s", v.Type, k)
		}
		r.resources[k] = &registeredResource{
			State:   &state,
			Pending: pending,
		}
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
	if v != nil {
		switch t := v.(type) {
		case bool, int, int32, int64, float32, float64, string:
			return t, nil
		case []interface{}:
			var xs []interface{}
			for _, x := range t {
				vv, err := r.evaluateUntypedExpression(x)
				if err != nil {
					return nil, err
				}
				xs = append(xs, vv)
			}
			return xs, nil
		case map[string]interface{}:
			m := make(map[string]interface{})
			for k, e := range t {
				vv, err := r.evaluateUntypedExpression(e)
				if err != nil {
					return nil, err
				}
				m[k] = vv
			}
			return r.evaluateMaybeFunctionExpression(m)
		default:
			return nil, errors.Errorf("unrecognized map element: %v", reflect.TypeOf(v))
		}
	}

	return nil, nil
}

// evaluateMaybeFunctionExpression takes a map which might include a function expression using the
// CloudFormation-style of builtin functions, and evaluates it. If it's not such a builtin function
// expression, the raw map value is simply returned as-is.
func (r *runner) evaluateMaybeFunctionExpression(v map[string]interface{}) (interface{}, error) {
	for k := range v {
		// Eliding Fn::GetAZs -- AWS specific.
		switch k {
		case "Ref":
			return r.evaluateBuiltinRef(v)
		case "Fn::Join":
			return r.evaluateBuiltinJoin(v)
		case "Fn::Select":
			return r.evaluateBuiltinSelect(v)
		case "Fn::GetAtt":
			return r.evaluateBuiltinGetAtt(v)
		case "Fn::FindInMap":
			return r.evaluateBuiltinBase64(v)
		case "Fn::Base64":
			return r.evaluateBuiltinBase64(v)
		case "Fn::If":
			return r.evaluateBuiltinBase64(v)
		case "Fn::ImportValue":
			return r.evaluateBuiltinImportValue(v)
		case "Fn::Invoke":
			return r.evaluateBuiltinInvoke(v)
		}
	}
	return v, nil
}

// evaluateBuiltinRef evaluates a "Ref" builtin. This map entry has a single value, which represents
// the resource name whose ID will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinRef(v map[string]interface{}) (interface{}, error) {
	k := v["Ref"]
	ks, ok := k.(string)
	if !ok {
		return nil, errors.Errorf("expected string resource name for Ref argument, got %v", reflect.TypeOf(k))
	}
	res, ok := r.resources[ks]
	if !ok {
		return nil, errors.Errorf("resource Ref named %s could not be found", ks)
	}
	return res.State.ID(), nil
}

func (r *runner) evaluateBuiltinJoin(v map[string]interface{}) (interface{}, error) {
	return nil, errors.New("NYI")
}

func (r *runner) evaluateBuiltinSelect(v map[string]interface{}) (interface{}, error) {
	return nil, errors.New("NYI")
}

// evaluateBuiltinGetAtt evaluates a "GetAtt" builtin. This map entry has a single two-valued array,
// the first value being the resource name, and the second being the property to read, and whose
// value will be looked up and substituted in its place.
func (r *runner) evaluateBuiltinGetAtt(v map[string]interface{}) (interface{}, error) {
	// Read and validate the arguments to Fn::GetAtt.
	att := v["Fn::GetAtt"]
	args, ok := att.([]interface{})
	if !ok {
		return nil, errors.Errorf(
			"expected Fn::GetAtt to contain a two-valued array, got %v", reflect.TypeOf(att))
	}
	if len(args) != 2 {
		return nil, errors.Errorf(
			"incorrect number of elements for Fn::GetAtt array; got %d, expected 2", len(args))
	}
	resourceName, ok := args[0].(string)
	if !ok {
		return nil, errors.Errorf(
			"expected first argument to Fn::GetAtt to be a resource name string; got %v", reflect.TypeOf(args[0]))
	}
	propertyName, ok := args[1].(string)
	if !ok {
		return nil, errors.Errorf(
			"expected second argument to Fn::GetAtt to be a property name string; got %v", reflect.TypeOf(args[1]))
	}

	// Look up the resource and ensure it exists.
	res, ok := r.resources[resourceName]
	if !ok {
		return nil, errors.Errorf("resource %s named by Fn::GetAtt could not be found", resourceName)
	}
	// HACKHACK: I had to add GetOutput, and make it lazily allocate, for this to work. Our registered
	//     resource types are untyped and so don't get pre-populated with all the right output properties.
	return res.Pending.GetOutput(propertyName), nil
}

func (r *runner) evaluateBuiltinFindInMap(v map[string]interface{}) (interface{}, error) {
	return nil, errors.New("NYI")
}

func (r *runner) evaluateBuiltinBase64(v map[string]interface{}) (interface{}, error) {
	return nil, errors.New("NYI")
}

func (r *runner) evaluateBuiltinIf(v map[string]interface{}) (interface{}, error) {
	return nil, errors.New("NYI")
}

func (r *runner) evaluateBuiltinImportValue(v map[string]interface{}) (interface{}, error) {
	return nil, errors.New("NYI")
}

// evaluateBuiltinInvoke evaluates the "Invoke" builtin, which enables templates to invoke arbitrary
// data source functions, to fetch information like the current availability zone, lookup AMIs, etc.
func (r *runner) evaluateBuiltinInvoke(v map[string]interface{}) (interface{}, error) {
	// Read and validate the arguments to Fn::Invoke.
	inv := v["Fn::Invoke"]
	invoke, ok := inv.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf(
			"expected Fn::Invoke to be a map containing the 'Function', 'Arguments', and a 'Return', got %v",
			reflect.TypeOf(inv))
	}
	fn := invoke["Function"]
	if fn == nil {
		return nil, errors.New("missing function name, Function, in the Fn::Invoke map")
	}
	tok, ok := fn.(string)
	if !ok {
		return nil, errors.Errorf(
			"expected function name, Function, in the Fn::Invoke map to be a string, got %v",
			reflect.TypeOf(fn))
	}
	var args map[string]interface{}
	argsmap := invoke["Arguments"]
	if argsmap != nil {
		// It's ok if arguments are missing, they are optional, but if present, we need a map.
		args, ok = argsmap.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf(
				"expected function args, Arguments, in the Fn::Invoke map to be a map, got %v",
				reflect.TypeOf(argsmap))
		}
	}
	ret := invoke["Return"]
	if ret == nil {
		// TODO: not clear this will be sufficiently expressive, or that it's even required!
		return nil, errors.New("missing return directive, Return, in the Fn::Invoke map")
	}
	rets, ok := ret.(string)
	if !ok {
		return nil, errors.Errorf(
			"expected return directive, Return, in the Fn::Invoke map to be a string, got %v",
			reflect.TypeOf(fn))
	}

	// The arguments may very well have expressions within them, so evaluate those now.
	for k, v := range args {
		nv, err := r.evaluateUntypedExpression(v)
		if err != nil {
			return nil, err
		}
		args[k] = nv
	}

	// At this point, we've got a function to invoke and some parameters! Invoke away.
	result := make(map[string]interface{})
	// HACKHACK: had to change Invoke to let maps through, not just structs per the previous code.
	if err := r.ctx.Invoke(tok, args, &result); err != nil {
		return nil, err
	}
	retv, ok := result[rets]
	if !ok {
		return nil, errors.Errorf(
			"Fn::Invoke of %s did not contain a property '%s' in the returned value", tok, rets)
	}
	return retv, nil
}

// untypedArgs is an untyped interface for a bag of properties.
type untypedArgs map[string]interface{}

func (untypedArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]interface{})(nil)).Elem()
}
