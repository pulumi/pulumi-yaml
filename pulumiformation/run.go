package pulumiformation

import (
	"encoding/json"
	"io/ioutil"
	"reflect"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
)

// MainTemplate is the assumed name of the JSON template file.
// TODO: would be nice to permit multiple files, but we'd need to know which is "main", and there's
//     no notion of "import" so we'd need to be a bit more clever. Might be nice to mimic e.g. Kustomize.
//     One idea is to hijack Pulumi.yaml's "main" directive and then just globally toposort the rest.
const MainTemplate = "Main.json"

// Load a template from the current working directory
func Load() (Template, error) {
	t := Template{}

	// Read in the template file (for now, hard coded as Main.*).
	b, err := ioutil.ReadFile(MainTemplate)
	if err != nil {
		return t, errors.Wrapf(err, "reading template %s", MainTemplate)
	}

	// Now decode the file using CloudFormation rules.
	// TODO: eventually this won't work because we'll need to customize (e.g., with !Invoke). Hopefully
	//     we can just fork rather than needing to recreate the CloudFormation oddities.
	if err = json.Unmarshal(b, &t); err != nil {
		return t, errors.Wrapf(err, "decoding template %s", MainTemplate)
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
	resources map[string]*lateboundCustomResourceState
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

func newRunner(ctx *pulumi.Context, t Template) *runner {
	return &runner{
		ctx:       ctx,
		t:         t,
		params:    make(map[string]interface{}),
		resources: make(map[string]*lateboundCustomResourceState),
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

		// Now register the resulting resource with the engine.
		state := lateboundCustomResourceState{name: k}
		err := r.ctx.RegisterResource(v.Type, k, untypedArgs(props), &state)
		if err != nil {
			return errors.Wrapf(err, "registering resource %s/%s", v.Type, k)
		}
		r.resources[k] = &state
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
	return res.ID(), nil
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

// untypedArgs is an untyped interface for a bag of properties.
type untypedArgs map[string]interface{}

func (untypedArgs) ElementType() reflect.Type {
	return reflect.TypeOf((*map[string]interface{})(nil)).Elem()
}
