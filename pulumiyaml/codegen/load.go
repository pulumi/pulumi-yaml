package codegen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

type importer struct {
	configuration   map[string]*model.Variable
	variables       map[string]*model.Variable
	stackReferences map[string]*model.Variable
	resources       map[string]*model.Variable
	outputs         map[string]*model.Variable
}

// null represents PCL's builtin `null` variable
var null = &model.Variable{
	Name:         "null",
	VariableType: model.NoneType,
}

// functions is the set of CFN functions.
var functions = codegen.NewStringSet(
	"Fn::Asset",
	"Fn::GetAtt",
	"Fn::Invoke",
	"Fn::Join",
	"Fn::Select",
	"Fn::StackReference",
	"Fn::Sub",
	"Ref")

// functionTags maps from tags that represent the short forms of functions to their corresponding function names.
var functionTags = map[string]string{
	"!Asset":          "Fn::Asset",
	"!GetAtt":         "Fn::GetAtt",
	"!Invoke":         "Fn::Invoke",
	"!Join":           "Fn::Join",
	"!Select":         "Fn::Select",
	"!StackReference": "Fn::StackReference",
	"!Sub":            "Fn::Sub",
	"!Ref":            "Ref",
}

// checkArgsArray validates the argument to a template function. If the argument is a sequence, its length is
// checked against the provided minimum and maximum arg count. If the sequence is valid, its values are returned.
//
// If the argument is a single value and the minimum count is greater than one, the argument is wrapped in an array
// and returned.
func (imp *importer) checkArgsArray(name string, v ast.Node, min, max int) ([]ast.Node, error) {
	if v.Type() != ast.SequenceType {
		if min > 1 {
			return nil, fmt.Errorf("the argument to '%s' must be an array", name)
		}
		return []ast.Node{v}, nil
	}

	arr := v.(*ast.SequenceNode).Values
	if len(arr) < min || (max >= 0 && len(arr) > max) {
		if min == max {
			return nil, fmt.Errorf("the argument to '%s' must have exactly %v elements", name, min)
		}
		if max >= 0 {
			return nil, fmt.Errorf("the argument to '%s' must have between %v and %v elements", name, min, max)
		}
		return nil, fmt.Errorf("the argument to '%s' must have at least %v elements", name, min)
	}
	return arr, nil
}

// importRef imports a reference to a config variable or resource. These entities all correspond to top-level variables
// in the Pulumi program, so each reference is imported as a scope traversal expression.
func (imp *importer) importRef(name string) (model.Expression, error) {
	v, ok := imp.configuration[name]
	if ok {
		return model.VariableReference(v), nil
	}
	if v, ok = imp.variables[name]; ok {
		return model.VariableReference(v), nil
	}

	v, ok = imp.resources[name]
	if !ok {
		return nil, fmt.Errorf("unknown config, variable, or resource '%v'", name)
	}
	return &model.ScopeTraversalExpression{
		Traversal: hcl.Traversal{hcl.TraverseRoot{Name: v.Name}, hcl.TraverseAttr{Name: "id"}},
		Parts:     []model.Traversable{v, model.DynamicType},
	}, nil
}

var holePattern = regexp.MustCompile(`\${([^}]*)}`)

func (imp *importer) importSubRef(ref string, environment map[string]model.Expression) (model.Expression, error) {
	if len(ref) == 0 {
		return nil, fmt.Errorf("empty variable in 'Fn::Sub' input string")
	}
	if parts := strings.Split(ref, "."); len(parts) > 1 {
		root, ok := imp.configuration[parts[0]]
		if !ok {
			if root, ok = imp.variables[parts[0]]; !ok {
				if root, ok = imp.resources[parts[0]]; !ok {
					return nil, fmt.Errorf("unknown config, variable, or resource '%v'", parts[0])
				}
			}
		}

		traversal, types := hcl.Traversal{hcl.TraverseRoot{Name: root.Name}}, []model.Traversable{root}
		for _, p := range parts[1:] {
			traversal, types = append(traversal, hcl.TraverseAttr{Name: p}), append(types, model.DynamicType)
		}
		return &model.ScopeTraversalExpression{
			Traversal: traversal,
			Parts:     types,
		}, nil
	}

	x, err := imp.importRef(ref)
	if err != nil {
		v, ok := environment[ref]
		if !ok {
			return nil, err
		}
		x = v
	}
	return x, nil
}

// importSub imports a call to Fn::Sub. The call is converted to a template expression. If an envrionment map is
// provided, references to map elements are replaced with the corresponding elements.
func (imp *importer) importSub(name string, value ast.Node) (model.Expression, error) {
	arr, err := imp.checkArgsArray(name, value, 1, 2)
	if err != nil {
		return nil, err
	}

	if arr[0].Type() != ast.StringType {
		return nil, fmt.Errorf("the first argument to 'Fn::Sub' must be a string")
	}

	var environment map[string]model.Expression
	if len(arr) == 2 {
		values, ok := mapValues(arr[1])
		if !ok {
			return nil, fmt.Errorf("the second argument to 'Fn::Sub' must be a mapping")
		}

		for _, f := range values {
			v, err := imp.importValue(f.Value)
			if err != nil {
				return nil, err
			}
			environment[keyString(f)] = v
		}
	}

	text := arr[0].(*ast.StringNode).Value
	holes := holePattern.FindAllStringSubmatchIndex(text, -1)

	var literals []string
	var refs []model.Expression
	start, end := 0, 0
	for _, hole := range holes {
		end = hole[0]

		literals = append(literals, text[start:end])

		ref, err := imp.importSubRef(text[hole[2]:hole[3]], environment)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)

		start = hole[1]
	}
	literals = append(literals, text[start:])

	contract.Assert(len(literals) == len(refs)+1)

	var parts []model.Expression
	for i, l := range literals {
		parts = append(parts, plainLit(l))
		if i < len(refs) {
			parts = append(parts, refs[i])
		}
	}
	return &model.TemplateExpression{Parts: parts}, nil
}

// importArgsArray validates and imports the argument to a function as an array of expressions.
func (imp *importer) importArgsArray(name string, arg ast.Node, min, max int) ([]model.Expression, error) {
	arr, err := imp.checkArgsArray(name, arg, min, max)
	if err != nil {
		return nil, err
	}

	var args []model.Expression
	for _, v := range arr {
		arg, err := imp.importValue(v)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return args, nil
}

// importJoin imports a call to Fn::Join. If the arguments to the join are a string literal and a sequence, the call
// is imported as a template expression. Otherwise, it is imported as a call to the `join` function.
func (imp *importer) importJoin(name string, arg ast.Node) (model.Expression, error) {
	args, err := imp.importArgsArray(name, arg, 2, 2)
	if err != nil {
		return nil, err
	}

	call := &model.FunctionCallExpression{
		Name: "join",
		Args: args,
	}

	// Turn `join("lit", [ ... ])` into a template expression.
	sep, ok := getStringValue(args[0])
	if !ok {
		return call, nil
	}

	tuple, ok := args[1].(*model.TupleConsExpression)
	if !ok {
		return call, nil
	}

	afterLiteral := false
	var parts []model.Expression
	for _, x := range tuple.Expressions {
		lit, ok := getStringValue(x)
		if ok {
			if afterLiteral {
				last := parts[len(parts)-1].(*model.LiteralValueExpression)
				last.Value = cty.StringVal(last.Value.AsString() + sep + lit)
			} else {
				if len(parts) > 0 {
					lit = sep + lit
				}
				parts = append(parts, plainLit(lit))
				afterLiteral = true
			}
		} else {
			if afterLiteral {
				last := parts[len(parts)-1].(*model.LiteralValueExpression)
				last.Value = cty.StringVal(last.Value.AsString() + sep + lit)
			} else {
				parts = append(parts, plainLit(sep))
			}
			parts = append(parts, x)
			afterLiteral = false
		}
	}
	if !afterLiteral {
		parts = append(parts, plainLit(""))
	}
	return &model.TemplateExpression{Parts: parts}, nil
}

// importFunctionCall imports a call to an AWS intrinsic function. The way the function is imported depends on the
// function:
//
// - `Fn::Asset` is imported as a call to the `fileAsset` function
// - `Fn::GetAtt` is imported as a scope traversal expression on the referenced resource to fetch the referenced
//   attribute
// - `Fn::Invoke` is imported as a call to `invoke`
// - `Fn::Join` is imported as either a template expression or a call to `join`
// - `Fn::Split` is imported as a call to `split`
// - `Fn::StackReference` is imported as a reference to the named stack
// - `Fn::Sub` is imported as a template expression
// - `Ref` is imported as a variable reference
//
func (imp *importer) importFunctionCall(name string, arg ast.Node) (model.Expression, error) {
	switch name {
	case "Fn::Asset":
		args, err := imp.importArgsArray(name, arg, 1, 1)
		if err != nil {
			return nil, err
		}
		return args[0], nil
	case "Fn::GetAtt":
		args, err := imp.importArgsArray(name, arg, 1, 2)
		if err != nil {
			return nil, err
		}

		var resourceName string
		var attrArg model.Expression
		if len(args) == 1 {
			attrPath, ok := getStringValue(args[0])
			if !ok {
				return nil, fmt.Errorf("the argument to 'Fn::GetAtt' must be a string")
			}
			dotIndex := strings.Index(attrPath, ".")
			if dotIndex == -1 {
				return nil, fmt.Errorf("attribute paths must be of the form \"resourceName.attrName\"")
			}
			resourceName, attrArg = attrPath[:dotIndex], plainLit(attrPath[dotIndex+1:])
		} else {
			rn, ok := getStringValue(args[0])
			if !ok {
				return nil, fmt.Errorf("the first argument to 'Fn::GetAtt' must be a string")
			}
			resourceName, attrArg = rn, args[1]
		}

		resourceVar, ok := imp.resources[resourceName]
		if !ok {
			return nil, fmt.Errorf("unknown resource '%v'", resourceName)
		}

		attrName, ok := getStringValue(attrArg)
		if !ok {
			return &model.IndexExpression{
				Collection: &model.ScopeTraversalExpression{
					Traversal: hcl.Traversal{
						hcl.TraverseRoot{Name: resourceVar.Name},
						hcl.TraverseAttr{Name: "attributes"},
					},
					Parts: []model.Traversable{
						resourceVar,
						model.DynamicType,
					},
				},
				Key: args[1],
			}, nil
		}

		return &model.ScopeTraversalExpression{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: resourceVar.Name},
				hcl.TraverseAttr{Name: "attributes"},
				hcl.TraverseAttr{Name: strings.Replace(attrName, ".", "", -1)},
			},
			Parts: []model.Traversable{
				resourceVar,
				model.DynamicType,
				model.DynamicType,
			},
		}, nil
	case "Fn::Invoke":
		args, err := imp.importArgsArray(name, arg, 1, 1)
		if err != nil {
			return nil, err
		}
		object, ok := args[0].(*model.ObjectConsExpression)
		if !ok {
			return nil, fmt.Errorf("the argument to Fn::Invoke must be an object")
		}

		function, ok := getObjectProperty(object, "Function")
		if !ok {
			return nil, fmt.Errorf("missing function name, Function, in the Fn::Invoke map")
		}
		invokeArgs := []model.Expression{function}

		if arguments, hasArgs := getObjectProperty(object, "Arguments"); hasArgs {
			invokeArgs = append(invokeArgs, arguments)
		}

		returnKey, ok := getObjectProperty(object, "Return")
		if !ok {
			return nil, fmt.Errorf("missing return directive, Return, in the Fn::Invoke map")
		}
		returnKeyString, ok := getStringValue(returnKey)
		if !ok {
			return nil, fmt.Errorf("expected return directive, Return, in the Fn::Invoke map to be a string")
		}

		return relativeTraversal(&model.FunctionCallExpression{
			Name: "invoke",
			Args: invokeArgs,
		}, returnKeyString), nil
	case "Fn::Join":
		return imp.importJoin(name, arg)
	case "Fn::Select":
		args, err := imp.importArgsArray(name, arg, 2, 2)
		if err != nil {
			return nil, err
		}

		indexStr, ok := getStringValue(args[0])
		if ok {
			indexInt, err := convert.Convert(cty.StringVal(indexStr), cty.Number)
			if err == nil {
				args[0] = &model.LiteralValueExpression{
					Value: indexInt,
				}
			} else {
				args[0] = quotedLit(indexStr)
			}
		}

		return &model.IndexExpression{
			Collection: args[1],
			Key:        args[0],
		}, nil
	case "Fn::StackReference":
		args, err := imp.importArgsArray(name, arg, 2, 2)
		if err != nil {
			return nil, err
		}
		stackName, ok := getStringValue(args[0])
		if !ok {
			return nil, fmt.Errorf("expected first argument to Fn::StackReference to be a stack name string")
		}
		stackVar := imp.stackReferences[stackName]

		outputName, ok := getStringValue(args[1])
		if !ok {
			return &model.IndexExpression{
				Collection: &model.ScopeTraversalExpression{
					Traversal: hcl.Traversal{
						hcl.TraverseRoot{Name: stackVar.Name},
						hcl.TraverseAttr{Name: "outputs"},
					},
					Parts: []model.Traversable{
						stackVar,
						model.DynamicType,
					},
				},
				Key: args[1],
			}, nil
		}

		return &model.ScopeTraversalExpression{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: stackVar.Name},
				hcl.TraverseAttr{Name: "outputs"},
				hcl.TraverseAttr{Name: outputName},
			},
			Parts: []model.Traversable{
				stackVar,
				model.DynamicType,
				model.DynamicType,
			},
		}, nil
	case "Fn::Sub":
		return imp.importSub(name, arg)
	case "Ref":
		arg, err := imp.importValue(arg)
		if err != nil {
			return nil, err
		}

		entityName, ok := getStringValue(arg)
		if !ok {
			return nil, fmt.Errorf("the argument to 'Ref' must be a string")
		}

		return imp.importRef(entityName)
	default:
		contract.Failf("unexpected function %v", name)
		return nil, nil
	}
}

// importValue imports an AST node that represents a YAML value as its equivalent PCL. Most nodes are imported as one
// would expect (e.g. sequences -> tuple construction, maps -> object construction, etc.). Function calls are the lone
// exception; see importFunction for more details.
func (imp *importer) importValue(node ast.Node) (model.Expression, error) {
	switch node := node.(type) {
	case *ast.SequenceNode:
		var expressions []model.Expression
		for _, v := range node.Values {
			e, err := imp.importValue(v)
			if err != nil {
				return nil, err
			}
			expressions = append(expressions, e)
		}
		return &model.TupleConsExpression{
			Expressions: expressions,
		}, nil
	case *ast.BoolNode:
		return &model.LiteralValueExpression{
			Value: cty.BoolVal(node.Value),
		}, nil
	case *ast.NullNode:
		return model.VariableReference(null), nil
	case *ast.FloatNode:
		return &model.LiteralValueExpression{
			Value: cty.NumberFloatVal(node.Value),
		}, nil
	case *ast.IntegerNode:
		var value cty.Value
		switch v := node.Value.(type) {
		case int64:
			value = cty.NumberIntVal(v)
		case uint64:
			value = cty.NumberUIntVal(v)
		default:
			contract.Failf("unexpected value of type %T in integer node", v)
		}
		return &model.LiteralValueExpression{Value: value}, nil
	case *ast.LiteralNode:
		return imp.importValue(node.Value)
	case *ast.MappingNode, *ast.MappingValueNode:
		values, ok := mapValues(node)
		contract.Assert(ok)

		if len(values) == 1 && functions.Has(keyString(values[0])) {
			return imp.importFunctionCall(keyString(values[0]), values[0].Value)
		}

		var items []model.ObjectConsItem
		for _, f := range values {
			v, err := imp.importValue(f.Value)
			if err != nil {
				return nil, err
			}
			items = append(items, objectConsItem(keyString(f), v))
		}
		return &model.ObjectConsExpression{
			Items: items,
		}, nil
	case *ast.StringNode:
		return imp.importSub("Fn::Sub", node)
	case *ast.TagNode:
		fnName, ok := functionTags[node.Start.Value]
		if !ok {
			return nil, fmt.Errorf("unknown tag %v", node.Start.Value)
		}
		return imp.importFunctionCall(fnName, node.Value)
	default:
		contract.Failf("unexpected YAML node of type %T", node)
		return nil, nil
	}
}

// importParameterType converts a YAML config variable type to its equivalent PCL type.
func importParameterType(s string) (string, bool) {
	switch s {
	case "String":
		return "string", true
	case "Number":
		return "number", true
	case "List<Number>":
		return "list(number)", true
	case "CommaDelimitedList", "List<String>":
		return "list(string)", true
	default:
		return "", false
	}
}

// importConfig imports a template config variable. The parameter is imported as a simple config variable definition.
func (imp *importer) importConfig(attr *ast.MappingValueNode) ([]model.BodyItem, error) {
	name := keyString(attr)
	values, ok := mapValues(attr.Value)
	if !ok {
		return nil, fmt.Errorf("config variable '%s' must be a mapping", name)
	}

	typ, ok := valueAt(values, "Type")
	if !ok {
		return nil, fmt.Errorf("config variable '%s' is missing required field 'Type'", name)
	}
	if typ.Type() != ast.StringType {
		return nil, fmt.Errorf("'Type' of config variable '%s' must be a string", name)
	}

	typeValue := typ.(*ast.StringNode).Value

	typeExpr, ok := importParameterType(typeValue)
	if !ok {
		return nil, fmt.Errorf("Unrecognized type '%v' for config variable '%s'", typeValue, name)
	}

	configVar, ok := imp.configuration[keyString(attr)]
	contract.Assert(ok)

	var defaultValue model.Expression
	if def, ok := valueAt(values, "Default"); ok {
		v, err := imp.importValue(def)
		if err != nil {
			return nil, err
		}
		defaultValue = v
	}

	// TODO(pdg): secret configuration -- requires changes in PCL

	configDef := &model.Block{
		Type:   "config",
		Labels: []string{configVar.Name, typeExpr},
		Body:   &model.Body{},
	}
	if defaultValue != nil {
		configDef.Body.Items = append(configDef.Body.Items, &model.Attribute{
			Name:  "default",
			Value: defaultValue,
		})
	}
	return []model.BodyItem{configDef}, nil
}

// importResource imports a YAML resource as a PCL resource.
func (imp *importer) importResource(attr *ast.MappingValueNode) (model.BodyItem, error) {
	name := keyString(attr)
	values, ok := mapValues(attr.Value)
	if !ok {
		return nil, fmt.Errorf("resource '%v' must be a mapping", name)
	}

	resourceVar, ok := imp.resources[name]
	contract.Assert(ok)

	var token string
	var items []model.BodyItem
	var optionItems []model.BodyItem
	for _, f := range values {
		switch strings.ToLower(keyString(f)) {
		case "type":
			if f.Value.Type() != ast.StringType {
				return nil, fmt.Errorf("the \"Type\" of reosurce '%v' must be a string", name)
			}
			token = resourceToken(f.Value.(*ast.StringNode).Value)
		case "properties":
			values, ok := mapValues(f.Value)
			if !ok {
				return nil, fmt.Errorf("the \"Properties\" of resource '%v' must be a mapping", name)
			}
			for _, f := range values {
				v, err := imp.importValue(f.Value)
				if err != nil {
					return nil, err
				}
				items = append(items, &model.Attribute{
					Name:  keyString(f),
					Value: v,
				})
			}
		case "ignorechanges", "protect":
			v, err := imp.importValue(f.Value)
			if err != nil {
				return nil, err
			}
			optionItems = append(optionItems, &model.Attribute{
				Name:  camel(keyString(f)),
				Value: v,
			})
		case "parent", "provider":
			str, ok := f.Value.(*ast.StringNode)
			if !ok {
				return nil, fmt.Errorf("the \"%s\" attrbute for resource '%v' must be a string", keyString(f), name)
			}
			resourceName := str.Value
			resourceVar, ok := imp.resources[resourceName]
			if !ok {
				return nil, fmt.Errorf("unknown resource '%v'", resourceName)
			}
			optionItems = append(optionItems, &model.Attribute{
				Name:  camel(keyString(f)),
				Value: model.VariableReference(resourceVar),
			})
		case "dependsn":
			var arr []ast.Node
			switch f.Value.Type() {
			case ast.StringType:
				arr = []ast.Node{f.Value}
			case ast.SequenceType:
				arr = f.Value.(*ast.SequenceNode).Values
			default:
				return nil, fmt.Errorf("the \"DependsOn\" attribute for resource '%v' must be a string or list of strings", name)
			}

			var refs []model.Expression
			for _, v := range arr {
				if v.Type() != ast.StringType {
					return nil, fmt.Errorf("the \"DependsOn\" attribute for resource '%v' must be a string or list of strings", name)
				}
				resourceName := v.(*ast.StringNode).Value
				resourceVar, ok := imp.resources[resourceName]
				if !ok {
					return nil, fmt.Errorf("unknown resource '%v'", resourceName)
				}
				refs = append(refs, model.VariableReference(resourceVar))
			}
			optionItems = append(optionItems, &model.Attribute{
				Name: "dependsOn",
				Value: &model.TupleConsExpression{
					Expressions: refs,
				},
			})
		case "additionalsecretoutputs", "aliases", "customtimeouts", "deletebeforereplace", "import", "version":
			return nil, fmt.Errorf("resource option '%v' is not yet implemented", f.Key)
		default:
			return nil, fmt.Errorf("unsupported property '%v' in resource '%v'", f.Key, name)
		}
	}

	if token == "" {
		return nil, fmt.Errorf("resource '%v' has no \"Type\" attribute", name)
	}
	return &model.Block{
		Type:   "resource",
		Labels: []string{resourceVar.Name, token},
		Body:   &model.Body{Items: items},
	}, nil
}

// importOutput imports a CloudFormation output as a PCL output.
func (imp *importer) importOutput(attr *ast.MappingValueNode) (model.BodyItem, error) {
	name := keyString(attr)

	outputVar, ok := imp.outputs[name]
	contract.Assert(ok)

	x, err := imp.importValue(attr.Value)
	if err != nil {
		return nil, err
	}

	return &model.Block{
		Type:   "output",
		Labels: []string{outputVar.Name},
		Body: &model.Body{
			Items: []model.BodyItem{
				&model.Attribute{
					Name:  "value",
					Value: x,
				},
			},
		},
	}, nil
}

// An objectImporter imports a mapping value node into a body item.
type objectImporter func(*ast.MappingValueNode) (model.BodyItem, error)

// importObjects is a helper that imports a set of named objects in a mapping. Each named object is passed to the
// provided importer.
func (imp *importer) importObjects(attr *ast.MappingValueNode, importer objectImporter) ([]model.BodyItem, error) {
	values, ok := mapValues(attr.Value)
	if !ok {
		return nil, fmt.Errorf("%s must be a mapping", keyString(attr))
	}

	var items []model.BodyItem
	for _, f := range values {
		i, err := importer(f)
		if err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, nil
}

// assignNames assigns names to the variables used to represent template configuration, outputs, and resources.
// Care is taken to keep configuration and output names as close to their original names as possible.
func (imp *importer) assignNames() {
	assigned := codegen.StringSet{}

	assign := func(name, suffix string) *model.Variable {
		assignName := func(name, suffix string) string {
			name = camel(makeLegalIdentifier(name))
			if !assigned.Has(name) {
				assigned.Add(name)
				return name
			}

			base := name + suffix
			name = base
			for i := 0; assigned.Has(name); i++ {
				name = fmt.Sprintf("%s%d", base, i)
			}
			assigned.Add(name)
			return name
		}

		return &model.Variable{
			Name:         assignName(name, suffix),
			VariableType: model.DynamicType,
		}
	}

	// TODO: do this in source order
	assignNames := func(m map[string]*model.Variable, suffix string) {
		names := make([]string, 0, len(m))
		for n := range m {
			names = append(names, n)
		}
		sort.Strings(names)

		for _, n := range names {
			m[n] = assign(n, suffix)
		}
	}

	assignNames(imp.configuration, "")
	assignNames(imp.outputs, "")
	assignNames(imp.variables, "Var")
	assignNames(imp.stackReferences, "Stack")
	assignNames(imp.resources, "Resource")
}

func (imp *importer) findFunctionStackReferences(name string, arg ast.Node) {
	imp.findStackReferences(arg)

	if name == "Fn::StackReference" {
		if seq, ok := arg.(*ast.SequenceNode); ok && len(seq.Values) > 0 {
			if stackName, ok := seq.Values[0].(*ast.StringNode); ok {
				imp.stackReferences[stackName.Value] = nil
			}
		}
	}
}

func (imp *importer) findStackReferences(node ast.Node) {
	switch node := node.(type) {
	case *ast.SequenceNode:
		for _, v := range node.Values {
			imp.findStackReferences(v)
		}
	case *ast.MappingNode, *ast.MappingValueNode:
		values, ok := mapValues(node)
		contract.Assert(ok)

		if len(values) == 1 && functions.Has(keyString(values[0])) {
			imp.findFunctionStackReferences(keyString(values[0]), values[0].Value)
		} else {
			for _, v := range values {
				imp.findStackReferences(v.Value)
			}
		}
	case *ast.TagNode:
		if fnName, ok := functionTags[node.Start.Value]; ok {
			imp.findFunctionStackReferences(fnName, node.Value)
		}
	}
}

func (imp *importer) importTemplate(file *ast.File) (*model.Body, error) {
	var root *ast.MappingNode
	switch len(file.Docs) {
	case 0:
		return &model.Body{}, nil
	case 1:
		body := file.Docs[0].Body
		if body.Type() != ast.MappingType {
			return nil, fmt.Errorf("template must be a mapping")
		}
		root = body.(*ast.MappingNode)
	default:
		return nil, fmt.Errorf("template must contain at most one document")
	}

	// Declare config variables, resources, and outputs.
	for _, f := range root.Values {
		var table map[string]*model.Variable
		switch strings.ToLower(keyString(f)) {
		case "configuration":
			table = imp.configuration
		case "variables":
			table = imp.variables
		case "resources":
			table = imp.resources
		case "outputs":
			table = imp.outputs
		default:
			continue
		}
		imp.findStackReferences(f.Value)
		if values, ok := mapValues(f.Value); ok {
			for _, f := range values {
				table[keyString(f)] = nil
			}
		}
	}
	imp.assignNames()

	var items []model.BodyItem
	for _, f := range root.Values {
		switch strings.ToLower(keyString(f)) {
		case "description":
			if f.Value.Type() != ast.StringType {
				return nil, fmt.Errorf("Description must be a string")
			}
			//comment = f.Value.(jsonast.String).String()
		case "configuration":
			values, ok := mapValues(f.Value)
			if !ok {
				return nil, fmt.Errorf("Parameters must be a map")
			}

			for _, f := range values {
				config, err := imp.importConfig(f)
				if err != nil {
					return nil, err
				}
				items = append(items, config...)
			}
		case "variables":
			values, ok := mapValues(f.Value)
			if !ok {
				return nil, fmt.Errorf("Parameters must be a map")
			}

			for _, f := range values {
				vv, ok := imp.variables[keyString(f)]
				contract.Assert(ok)

				v, err := imp.importValue(f.Value)
				if err != nil {
					return nil, err
				}
				items = append(items, &model.Attribute{
					Name:  vv.Name,
					Value: v,
				})
			}

		case "resources":
			resources, err := imp.importObjects(f, imp.importResource)
			if err != nil {
				return nil, err
			}
			items = append(items, resources...)
		case "outputs":
			outputs, err := imp.importObjects(f, imp.importOutput)
			if err != nil {
				return nil, err
			}
			items = append(items, outputs...)
		}
	}

	body := &model.Body{Items: items}
	formatBody(body)
	return body, nil

}

func ImportTemplate(file *ast.File) (*model.Body, error) {
	imp := importer{
		configuration:   map[string]*model.Variable{},
		variables:       map[string]*model.Variable{},
		stackReferences: map[string]*model.Variable{},
		resources:       map[string]*model.Variable{},
		outputs:         map[string]*model.Variable{},
	}
	return imp.importTemplate(file)
}

func ImportFile(path string) (*model.Body, error) {
	file, err := parser.ParseFile(path, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %v: %v", path, err)
	}
	return ImportTemplate(file)
}
