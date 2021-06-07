package codegen

import (
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi/pulumi-yaml/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pulumiyaml/syntax"
)

type importer struct {
	referencedStacks []string

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

// importRef imports a reference to a config variable or resource. These entities all correspond to top-level variables
// in the Pulumi program, so each reference is imported as a scope traversal expression.
func (imp *importer) importRef(node ast.Expr, name string, environment map[string]model.Expression, isAccess bool) (model.Expression, syntax.Diagnostics) {
	if v, ok := imp.configuration[name]; ok {
		return model.VariableReference(v), nil
	}
	if v, ok := imp.variables[name]; ok {
		return model.VariableReference(v), nil
	}
	if v, ok := imp.resources[name]; ok {
		if isAccess {
			return model.VariableReference(v), nil
		}
		return &model.ScopeTraversalExpression{
			Traversal: hcl.Traversal{hcl.TraverseRoot{Name: v.Name}, hcl.TraverseAttr{Name: "id"}},
			Parts:     []model.Traversable{v, model.DynamicType},
		}, nil
	}
	if x, ok := environment[name]; ok {
		return x, nil
	}

	return &model.ScopeTraversalExpression{
		Traversal: hcl.Traversal{hcl.TraverseRoot{Name: name}},
		Parts:     []model.Traversable{model.DynamicType},
	}, syntax.Diagnostics{ast.ExprError(node, fmt.Sprintf("unknown config, variable, or resource '%v'", name), "")}
}

func (imp *importer) importPropertyAccess(node ast.Expr, access *ast.PropertyAccess, environment map[string]model.Expression) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	accessors := access.Accessors

	receiver, rdiags := imp.importRef(node, accessors[0].(*ast.PropertyName).Name, environment, len(accessors) > 1)
	diags.Extend(rdiags...)

	traversal, parts := hcl.Traversal{}, []model.Traversable{model.DynamicType}
	for accessors = accessors[1:]; len(accessors) != 0; accessors = accessors[1:] {
		switch accessor := accessors[0].(type) {
		case *ast.PropertyName:
			traversal = append(traversal, hcl.TraverseAttr{Name: accessor.Name})
		case *ast.PropertySubscript:
			switch index := accessor.Index.(type) {
			case string:
				traversal = append(traversal, hcl.TraverseAttr{Name: index})
			case int:
				traversal = append(traversal, hcl.TraverseIndex{Key: cty.NumberIntVal(int64(index))})
			}
		}
	}

	return &model.RelativeTraversalExpression{
		Source:    receiver,
		Traversal: traversal,
		Parts:     parts,
	}, diags
}

// importInterpolate imports a string interpolation. The call is converted to a template expression. If an envrionment map is
// provided, references to map elements are replaced with the corresponding elements.
func (imp *importer) importInterpolate(node *ast.InterpolateExpr, substitutions *ast.ObjectExpr) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	var environment map[string]model.Expression
	if substitutions != nil {
		environment := map[string]model.Expression{}
		for _, kvp := range substitutions.Entries {
			v, vdiags := imp.importExpr(kvp.Value)
			diags.Extend(vdiags...)

			environment[kvp.Key.(*ast.StringExpr).Value] = v
		}
	}

	var parts []model.Expression
	for _, part := range node.Parts {
		parts = append(parts, plainLit(part.Text))

		if part.Value != nil {
			ref, rdiags := imp.importPropertyAccess(node, part.Value, environment)
			diags.Extend(rdiags...)

			parts = append(parts, ref)
		}
	}
	if len(parts) > 0 {
		if _, isLit := parts[len(parts)-1].(*model.LiteralValueExpression); !isLit {
			parts = append(parts, plainLit(""))
		}
	}

	return &model.TemplateExpression{Parts: parts}, diags
}

// importJoin imports a call to Fn::Join as a call to the `join` function.
func (imp *importer) importJoin(node *ast.JoinExpr) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	delim, ddiags := imp.importExpr(node.Delimiter)
	diags.Extend(ddiags...)

	values, vdiags := imp.importExpr(node.Values)
	diags.Extend(vdiags...)

	if diags.HasErrors() {
		return nil, diags
	}

	call := &model.FunctionCallExpression{
		Name: "join",
		Args: []model.Expression{delim, values},
	}
	return call, diags
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
func (imp *importer) importBuiltin(node ast.BuiltinExpr) (model.Expression, syntax.Diagnostics) {
	switch node := node.(type) {
	case *ast.AssetExpr:
		path, pdiags := imp.importExpr(node.Path)
		switch node.Kind.Value {
		case "File":
			return &model.FunctionCallExpression{
				Name: "fileAsset",
				Args: []model.Expression{path},
			}, pdiags
		case "String":
			return &model.FunctionCallExpression{
				Name: "stringAsset",
				Args: []model.Expression{path},
			}, pdiags
		case "Remote":
			return &model.FunctionCallExpression{
				Name: "remoteAsset",
				Args: []model.Expression{path},
			}, pdiags
		default:
			contract.Failf("unrecognized asset kind %v", node.Kind.Value)
			return nil, nil
		}
	case *ast.GetAttExpr:
		resourceName, propertyName := node.ResourceName.Value, node.PropertyName.Value

		resourceVar, ok := imp.resources[resourceName]
		if !ok {
			return nil, syntax.Diagnostics{ast.ExprError(node.ResourceName, fmt.Sprintf("unknown resource '%v'", resourceName), "")}
		}

		return &model.ScopeTraversalExpression{
			Traversal: hcl.Traversal{
				hcl.TraverseRoot{Name: resourceVar.Name},
				hcl.TraverseAttr{Name: "attributes"},
				hcl.TraverseAttr{Name: propertyName},
			},
			Parts: []model.Traversable{
				resourceVar,
				model.DynamicType,
				model.DynamicType,
			},
		}, nil
	case *ast.InvokeExpr:
		var diags syntax.Diagnostics

		function, fdiags := imp.importExpr(node.Token)
		diags.Extend(fdiags...)

		invokeArgs := []model.Expression{function}

		if node.CallArgs != nil {
			args, adiags := imp.importExpr(node.CallArgs)
			diags.Extend(adiags...)

			invokeArgs = append(invokeArgs, args)
		}

		return relativeTraversal(&model.FunctionCallExpression{
			Name: "invoke",
			Args: invokeArgs,
		}, node.Return.Value), diags
	case *ast.JoinExpr:
		return imp.importJoin(node)
	case *ast.SelectExpr:
		var diags syntax.Diagnostics

		index, idiags := imp.importExpr(node.Index)
		diags.Extend(idiags...)

		values, vdiags := imp.importExpr(node.Values)
		diags.Extend(vdiags...)

		return &model.IndexExpression{
			Collection: values,
			Key:        index,
		}, diags
	case *ast.StackReferenceExpr:
		stackName := node.StackName.Value
		stackVar := imp.stackReferences[stackName]

		propertyName, diags := imp.importExpr(node.PropertyName)

		if str, ok := propertyName.(*model.LiteralValueExpression); ok {
			return &model.ScopeTraversalExpression{
				Traversal: hcl.Traversal{
					hcl.TraverseRoot{Name: stackVar.Name},
					hcl.TraverseAttr{Name: "outputs"},
					hcl.TraverseAttr{Name: str.Value.AsString()},
				},
				Parts: []model.Traversable{
					stackVar,
					model.DynamicType,
					model.DynamicType,
				},
			}, diags
		}

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
			Key: propertyName,
		}, diags
	case *ast.SubExpr:
		return imp.importInterpolate(node.Interpolate, node.Substitutions)
	case *ast.RefExpr:
		return imp.importRef(node.ResourceName, node.ResourceName.Value, nil, false)
	default:
		contract.Failf("unexpected builtin type %T", node)
		return nil, nil
	}
}

// importExpr imports an AST expression as its equivalent PCL. Most nodes are imported as one would expect (e.g.
// sequences -> tuple construction, maps -> object construction, etc.). Function calls are the lone exception; see
// importFunction for more details.
func (imp *importer) importExpr(node ast.Expr) (model.Expression, syntax.Diagnostics) {
	switch node := node.(type) {
	case *ast.NullExpr:
		return model.VariableReference(null), nil
	case *ast.BooleanExpr:
		return &model.LiteralValueExpression{
			Value: cty.BoolVal(node.Value),
		}, nil
	case *ast.NumberExpr:
		return &model.LiteralValueExpression{
			Value: cty.NumberFloatVal(node.Value),
		}, nil
	case *ast.StringExpr:
		return quotedLit(node.Value), nil
	case *ast.ListExpr:
		var diags syntax.Diagnostics
		var expressions []model.Expression
		for _, v := range node.Elements {
			e, ediags := imp.importExpr(v)
			diags.Extend(ediags...)

			expressions = append(expressions, e)
		}
		return &model.TupleConsExpression{
			Expressions: expressions,
		}, diags
	case *ast.ObjectExpr:
		var diags syntax.Diagnostics
		var items []model.ObjectConsItem
		for _, kvp := range node.Entries {
			k, kdiags := imp.importExpr(kvp.Key)
			diags.Extend(kdiags...)

			v, vdiags := imp.importExpr(kvp.Value)
			diags.Extend(vdiags...)

			items = append(items, model.ObjectConsItem{Key: k, Value: v})
		}
		return &model.ObjectConsExpression{
			Items: items,
		}, diags
	case *ast.InterpolateExpr:
		return imp.importInterpolate(node, nil)
	case *ast.SymbolExpr:
		return imp.importPropertyAccess(node, node.Property, nil)
	case ast.BuiltinExpr:
		return imp.importBuiltin(node)
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
func (imp *importer) importConfig(kvp ast.ConfigMapEntry) (model.BodyItem, syntax.Diagnostics) {
	name, config := kvp.Key.Value, kvp.Value

	typeExpr, ok := importParameterType(config.Type.Value)
	if !ok {
		return nil, syntax.Diagnostics{ast.ExprError(config.Type, fmt.Sprintf("unrecognized type '%v' for config variable '%s'", config.Type.Value, name), "")}
	}

	configVar, ok := imp.configuration[name]
	contract.Assert(ok)

	var defaultValue model.Expression
	if config.Default != nil {
		v, diags := imp.importExpr(config.Default)
		if diags.HasErrors() {
			return nil, diags
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
	return configDef, nil
}

// importResource imports a YAML resource as a PCL resource.
func (imp *importer) importResource(kvp ast.ResourcesMapEntry) (model.BodyItem, syntax.Diagnostics) {
	name, resource := kvp.Key.Value, kvp.Value

	resourceVar, ok := imp.resources[name]
	contract.Assert(ok)

	token := resourceToken(resource.Type.Value)

	var diags syntax.Diagnostics
	var items []model.BodyItem
	for _, kvp := range resource.Properties.GetEntries() {
		v, vdiags := imp.importExpr(kvp.Value)
		diags.Extend(vdiags...)

		items = append(items, &model.Attribute{
			Name:  kvp.Key.Value,
			Value: v,
		})
	}

	// TODO: resource options not supported by PCL: component, additional secret outputs, aliases, custom timeouts, delete before replace, import, version

	var optionItems []model.BodyItem
	if len(resource.DependsOn.GetElements()) != 0 {
		var refs []model.Expression
		for _, v := range resource.DependsOn.Elements {
			resourceName := v.Value
			if resourceVar, ok := imp.resources[resourceName]; ok {
				refs = append(refs, model.VariableReference(resourceVar))
			} else {
				diags.Extend(ast.ExprError(v, fmt.Sprintf("unknown resource '%v'", resourceName), ""))
			}
		}
		optionItems = append(optionItems, &model.Attribute{
			Name: "dependsOn",
			Value: &model.TupleConsExpression{
				Expressions: refs,
			},
		})
	}
	if len(resource.IgnoreChanges.GetElements()) != 0 {
		var paths []model.Expression
		for _, v := range resource.IgnoreChanges.Elements {
			paths = append(paths, plainLit(v.Value))
		}
		optionItems = append(optionItems, &model.Attribute{
			Name:  "ignoreChanges",
			Value: &model.TupleConsExpression{Expressions: paths},
		})
	}
	if resource.Parent != nil && resource.Parent.Value != "" {
		resourceName := resource.Parent.Value
		if resourceVar, ok := imp.resources[resourceName]; ok {
			optionItems = append(optionItems, &model.Attribute{
				Name:  "parent",
				Value: model.VariableReference(resourceVar),
			})
		} else {
			diags.Extend(ast.ExprError(resource.Parent, fmt.Sprintf("unknown resource '%v'", resourceName), ""))
		}
	}
	if resource.Protect != nil && resource.Protect.Value {
		optionItems = append(optionItems, &model.Attribute{
			Name:  "protect",
			Value: &model.LiteralValueExpression{Value: cty.BoolVal(resource.Protect.Value)},
		})
	}
	if resource.Provider != nil && resource.Provider.Value != "" {
		resourceName := resource.Parent.Value
		if resourceVar, ok := imp.resources[resourceName]; ok {
			optionItems = append(optionItems, &model.Attribute{
				Name:  "provider",
				Value: model.VariableReference(resourceVar),
			})
		} else {
			diags.Extend(ast.ExprError(resource.Parent, fmt.Sprintf("unknown resource '%v'", resourceName), ""))
		}
	}

	r := &model.Block{
		Type:   "resource",
		Labels: []string{resourceVar.Name, token},
		Body:   &model.Body{Items: items},
	}

	// TODO: comments
	//	if comment := attr.Key.GetComment(); comment != nil {
	//		r.Tokens = hclsyntax.NewBlockTokens(r.Type, r.Labels...)
	//		r.Tokens.Type.LeadingTrivia = hclsyntax.TriviaList{hclsyntax.NewComment(strings.Split(strings.TrimSpace(comment.Value), "\n"))}
	//	}

	return r, diags
}

// importOutput imports a CloudFormation output as a PCL output.
func (imp *importer) importOutput(kvp ast.PropertyMapEntry) (model.BodyItem, syntax.Diagnostics) {
	name := kvp.Key.Value

	outputVar, ok := imp.outputs[name]
	contract.Assert(ok)

	x, diags := imp.importExpr(kvp.Value)

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
	}, diags
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

func (imp *importer) findStackReferences(node ast.Expr) {
	switch node := node.(type) {
	case *ast.ListExpr:
		for _, v := range node.Elements {
			imp.findStackReferences(v)
		}
	case *ast.ObjectExpr:
		for _, kvp := range node.Entries {
			imp.findStackReferences(kvp.Value)
		}
	case *ast.StackReferenceExpr:
		if _, ok := imp.stackReferences[node.StackName.Value]; !ok {
			imp.referencedStacks = append(imp.referencedStacks, node.StackName.Value)
			imp.stackReferences[node.StackName.Value] = nil
		}
	case ast.BuiltinExpr:
		imp.findStackReferences(node.Args())
	}
}

func (imp *importer) importTemplate(file *ast.TemplateDecl) (*model.Body, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	// Declare config variables, resources, and outputs.
	for _, kvp := range file.Configuration.GetEntries() {
		imp.configuration[kvp.Key.Value] = nil
	}
	for _, kvp := range file.Resources.GetEntries() {
		for _, kvp := range kvp.Value.Properties.GetEntries() {
			imp.findStackReferences(kvp.Value)
		}
		imp.resources[kvp.Key.Value] = nil
	}
	for _, kvp := range file.Outputs.GetEntries() {
		imp.findStackReferences(kvp.Value)
		imp.outputs[kvp.Key.Value] = nil
	}
	imp.assignNames()

	var items []model.BodyItem

	// Import config.
	for _, kvp := range file.Configuration.GetEntries() {
		config, cdiags := imp.importConfig(kvp)
		diags.Extend(cdiags...)

		if config != nil {
			items = append(items, config)
		}
	}

	// Import stack references.
	//
	// TODO: this isn't supported by PCL.
	for _, name := range imp.referencedStacks {
		stackVar, ok := imp.stackReferences[name]
		contract.Assert(ok)

		items = append(items, &model.Block{
			Type:   "stackReference",
			Labels: []string{stackVar.Name, name},
			Body:   &model.Body{},
		})
	}

	// Import resources.
	for _, kvp := range file.Resources.GetEntries() {
		resource, rdiags := imp.importResource(kvp)
		diags.Extend(rdiags...)

		if resource != nil {
			items = append(items, resource)
		}
	}

	// Import outputs.
	for _, kvp := range file.Outputs.GetEntries() {
		output, odiags := imp.importOutput(kvp)
		diags.Extend(odiags...)

		if output != nil {
			items = append(items, output)
		}
	}

	body := &model.Body{Items: items}
	formatBody(body)
	return body, diags

}

// ImportTemplate converts a YAML template to a PCL definition.
func ImportTemplate(file *ast.TemplateDecl) (*model.Body, syntax.Diagnostics) {
	imp := importer{
		configuration:   map[string]*model.Variable{},
		variables:       map[string]*model.Variable{},
		stackReferences: map[string]*model.Variable{},
		resources:       map[string]*model.Variable{},
		outputs:         map[string]*model.Variable{},
	}
	return imp.importTemplate(file)
}
