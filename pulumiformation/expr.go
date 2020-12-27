package pulumiformation

import (
	"reflect"

	"github.com/pkg/errors"
)

// Expr is an expression in the Pulumiformation language
type Expr interface {
	isExpr()
}

// Value is a concrete value of type bool, int, int32, int64, float32, float64 or string.
type Value struct {
	Val interface{}
}

func (*Value) isExpr() {}

// Array is an array of expressions.
type Array struct {
	Elems []Expr
}

func (*Array) isExpr() {}

// Object is an map of strings to expressions.
type Object struct {
	Elems map[string]Expr
}

func (*Object) isExpr() {}

// Ref is a function expression that computes a reference to another resource.
type Ref struct {
	ResourceName string
}

func (*Ref) isExpr() {}

// GetAtt is a function expression that accesses an output property of another resources.
type GetAtt struct {
	ResourceName string
	PropertyName string
}

func (*GetAtt) isExpr() {}

// Invoke is a function expression that invokes a Pulumi function by type token.
type Invoke struct {
	Token  string
	Args   *Object
	Return string
}

func (*Invoke) isExpr() {}

// Parse a given untyped expression value from a template into an Expr object
func Parse(v interface{}) (Expr, error) {
	if v == nil {
		return nil, nil
	}

	switch t := v.(type) {
	case bool, int, int32, int64, float32, float64, string:
		return &Value{Val: t}, nil
	case []interface{}:
		var elems []Expr
		for _, x := range t {
			vv, err := Parse(x)
			if err != nil {
				return nil, err
			}
			elems = append(elems, vv)
		}
		return &Array{Elems: elems}, nil
	case map[string]interface{}:
		elems := make(map[string]Expr)
		for k, e := range t {
			vv, err := Parse(e)
			if err != nil {
				return nil, err
			}
			elems[k] = vv
		}
		// Check if this is a function, and if so return the parsed function.
		for k := range elems {
			switch k {
			case "Ref":
				return parseRef(elems)
			case "Fn::GetAtt":
				return parseGetAtt(elems)
			case "Fn::Invoke":
				return parseInvoke(elems)
				// case "Fn::Join":
				// 	return r.evaluateBuiltinJoin(v)
				// case "Fn::Select":
				// 	return r.evaluateBuiltinSelect(v)
				// case "Fn::FindInMap":
				// 	return r.evaluateBuiltinFindInMap(v)
				// case "Fn::Base64":
				// 	return r.evaluateBuiltinBase64(v)
				// case "Fn::If":
				// 	return r.evaluateBuiltinIf(v)
				// case "Fn::ImportValue":
				// 	return r.evaluateBuiltinImportValue(v)
			}
		}
		// Else return the object directly.
		return &Object{Elems: elems}, nil
	default:
		return nil, errors.Errorf("unrecognized map element: %v", reflect.TypeOf(v))
	}
}

// parseRef reads and validates the arguments to Ref.
func parseRef(v map[string]Expr) (*Ref, error) {
	k := v["Ref"]
	kv, ok := k.(*Value)
	if !ok {
		return nil, errors.Errorf("expected string resource name for Ref argument, got %v", reflect.TypeOf(k))
	}
	ks, ok := kv.Val.(string)
	if !ok {
		return nil, errors.Errorf("expected string resource name for Ref argument, got %v", reflect.TypeOf(kv))
	}
	return &Ref{ResourceName: ks}, nil
}

// parseGetAtt reads and validates the arguments to Fn::GetAtt.
func parseGetAtt(v map[string]Expr) (*GetAtt, error) {
	att := v["Fn::GetAtt"]
	arr, ok := att.(*Array)
	if !ok {
		return nil, errors.Errorf(
			"expected Fn::GetAtt to contain a two-valued array, got %v", reflect.TypeOf(att))
	}
	args := arr.Elems
	if len(args) != 2 {
		return nil, errors.Errorf(
			"incorrect number of elements for Fn::GetAtt array; got %d, expected 2", len(args))
	}
	resourceNameV, ok := args[0].(*Value)
	if !ok {
		return nil, errors.Errorf(
			"expected first argument to Fn::GetAtt to be a resource name string; got %v", reflect.TypeOf(args[0]))
	}
	resourceName, ok := resourceNameV.Val.(string)
	if !ok {
		return nil, errors.Errorf(
			"expected first argument to Fn::GetAtt to be a resource name string; got %v", reflect.TypeOf(args[0]))
	}
	propertyNameV, ok := args[1].(*Value)
	if !ok {
		return nil, errors.Errorf(
			"expected second argument to Fn::GetAtt to be a property name string; got %v", reflect.TypeOf(args[1]))
	}
	propertyName, ok := propertyNameV.Val.(string)
	if !ok {
		return nil, errors.Errorf(
			"expected second argument to Fn::GetAtt to be a property name string; got %v", reflect.TypeOf(args[1]))
	}
	return &GetAtt{
		ResourceName: resourceName,
		PropertyName: propertyName,
	}, nil
}

func parseInvoke(v map[string]Expr) (*Invoke, error) {
	// Read and validate the arguments to Fn::Invoke.
	inv := v["Fn::Invoke"]
	invoke, ok := inv.(*Object)
	if !ok {
		return nil, errors.Errorf(
			"expected Fn::Invoke to be a map containing the 'Function', 'Arguments', and a 'Return', got %v",
			reflect.TypeOf(inv))
	}
	fn := invoke.Elems["Function"]
	if fn == nil {
		return nil, errors.New("missing function name, Function, in the Fn::Invoke map")
	}
	tokV, ok := fn.(*Value)
	if !ok {
		return nil, errors.Errorf(
			"expected function name, Function, in the Fn::Invoke map to be a string, got %v",
			reflect.TypeOf(fn))
	}
	tok, ok := tokV.Val.(string)
	if !ok {
		return nil, errors.Errorf(
			"expected function name, Function, in the Fn::Invoke map to be a string, got %v",
			reflect.TypeOf(fn))
	}
	var args *Object
	argsmap := invoke.Elems["Arguments"]
	if argsmap != nil {
		// It's ok if arguments are missing, they are optional, but if present, we need a map.
		args, ok = argsmap.(*Object)
		if !ok {
			return nil, errors.Errorf(
				"expected function args, Arguments, in the Fn::Invoke map to be a map, got %v",
				reflect.TypeOf(argsmap))
		}
	}
	ret := invoke.Elems["Return"]
	if ret == nil {
		// TODO: not clear this will be sufficiently expressive, or that it's even required!
		return nil, errors.New("missing return directive, Return, in the Fn::Invoke map")
	}
	retsV, ok := ret.(*Value)
	if !ok {
		return nil, errors.Errorf(
			"expected return directive, Return, in the Fn::Invoke map to be a string, got %v",
			reflect.TypeOf(fn))
	}
	rets, ok := retsV.Val.(string)
	if !ok {
		return nil, errors.Errorf(
			"expected return directive, Return, in the Fn::Invoke map to be a string, got %v",
			reflect.TypeOf(fn))
	}
	return &Invoke{
		Token:  tok,
		Args:   args,
		Return: rets,
	}, nil
}

// GetResourceDependencies gets the full set of implicit and explicit dependencies for a Resource.
func GetResourceDependencies(r *Resource) ([]string, error) {
	var deps []string
	for _, v := range r.Properties {
		e, err := Parse(v)
		if err != nil {
			return nil, err
		}
		deps = append(deps, getExpressionDependencies(e)...)
	}
	deps = append(deps, r.DependsOn...)
	return deps, nil
}

// getResourceDependencies gets the resource dependencies of an expression.
func getExpressionDependencies(e Expr) []string {
	var deps []string
	switch t := e.(type) {
	case *Value:
		// Nothing
	case *Array:
		for _, x := range t.Elems {
			deps = append(deps, getExpressionDependencies(x)...)
		}
	case *Object:
		for _, x := range t.Elems {
			deps = append(deps, getExpressionDependencies(x)...)
		}
	case *Ref:
		deps = append(deps, t.ResourceName)
	case *GetAtt:
		deps = append(deps, t.ResourceName)
	case *Invoke:
		deps = append(deps, getExpressionDependencies(t.Args)...)
	default:
		panic("fatal: invalid expr type")
	}
	return deps
}
