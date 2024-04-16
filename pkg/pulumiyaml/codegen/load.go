// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/config"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

type importer struct {
	referencedStacks []string

	loader          pulumiyaml.PackageLoader
	configuration   map[string]*model.Variable
	variables       map[string]*model.Variable
	stackReferences map[string]*model.Variable
	resources       map[string]*model.Variable
	outputs         map[string]*model.Variable
}

type packageInfo struct {
	version           string
	pluginDownloadURL string
}

// null represents PCL's builtin `null` variable
var null = &model.Variable{
	Name:         "null",
	VariableType: model.NoneType,
}

// importRef imports a reference to a config variable or resource. These entities all correspond to top-level variables
// in the Pulumi program, so each reference is imported as a scope traversal expression.
func (imp *importer) importRef(node ast.Expr, name string, environment map[string]model.Expression, isAccess bool, hint schema.Type) (model.Expression, syntax.Diagnostics) {
	// `pulumi` is not a real variable, so it doesn't make sense to look it up.
	contract.Assertf(name != pulumiyaml.PulumiVarName, "%[1]T: %[1]v", node)
	if v, ok := imp.configuration[name]; ok {
		return model.VariableReference(v), nil
	}
	if v, ok := imp.variables[name]; ok {
		return model.VariableReference(v), nil
	}
	if v, ok := imp.resources[name]; ok {
		// TODO: This comparison is not 100% sound, since hint may be a union that
		// includes a `string` type but does not include the resource type itself.
		//
		// The solution is to expose IsAssignable from pulumiyaml.Typing, but for that to
		// be reliable we would need to harden a lot of our top-sort algorithm against
		// missing nodes.
		exportID := codegen.UnwrapType(hint) == schema.StringType
		if isAccess || !exportID {
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
		Traversal: hcl.Traversal{hcl.TraverseRoot{Name: camel(makeLegalIdentifier(name))}},
		Parts:     []model.Traversable{model.DynamicType},
	}, syntax.Diagnostics{ast.ExprError(node, fmt.Sprintf("unknown config, variable, or resource '%v'", name), "")}
}

// Handles the special object `pulumi` injected into the global namespace of YAML programs.
func (imp *importer) pulumiPropertyAccess(node ast.Expr, accessors []ast.PropertyAccessor) (model.Expression, bool, syntax.Diagnostics) {
	wrapDiag := func(msg string, args ...interface{}) syntax.Diagnostics {
		return syntax.Diagnostics{ast.ExprError(node, fmt.Sprintf(msg, args...), "")}
	}
	if len(accessors) == 0 {
		// Invalid SymbolExpr
		return nil, false, wrapDiag("Cannot have empty variables")
	}
	if name, ok := accessors[0].(*ast.PropertyName); !ok || name.Name != pulumiyaml.PulumiVarName {
		// Not the `pulumi` symbol, so return
		return nil, false, nil
	}
	if len(accessors) == 1 {
		return nil, false, wrapDiag("`pulumi` is a special variable, and cannot be passed around in transpiled code.")
	}
	prop, ok := accessors[1].(*ast.PropertyName)
	if !ok {
		return nil, true, wrapDiag("cannot index into the `pulumi` variable: %v", node)
	}
	simple := model.StaticFunctionSignature{ReturnType: model.StringType}
	switch prop.Name {
	case "cwd":
		return &model.FunctionCallExpression{
			Name:      "cwd",
			Signature: simple,
		}, true, nil
	case "project":
		return &model.FunctionCallExpression{
			Name:      "project",
			Signature: simple,
		}, true, nil
	case "stack":
		return &model.FunctionCallExpression{
			Name:      "stack",
			Signature: simple,
		}, true, nil
	default:
		return nil, true, wrapDiag("Unknown property of the `pulumi` variable: '%s'", prop.Name)
	}
}

func (imp *importer) importPropertyAccess(node ast.Expr, access *ast.PropertyAccess, environment map[string]model.Expression, hint schema.Type) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	accessors := access.Accessors

	if f, ok, diags := imp.pulumiPropertyAccess(node, accessors); ok {
		return f, diags
	}

	receiver, rdiags := imp.importRef(node, accessors[0].(*ast.PropertyName).Name, environment, len(accessors) > 1, hint)
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

// importInterpolate imports a string interpolation. The call is converted to a template expression. If an environment map is
// provided, references to map elements are replaced with the corresponding elements.
func (imp *importer) importInterpolate(node *ast.InterpolateExpr, substitutions *ast.ObjectExpr) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	var environment map[string]model.Expression
	if substitutions != nil {
		environment := map[string]model.Expression{}
		for _, kvp := range substitutions.Entries {
			v, vdiags := imp.importExpr(kvp.Value, nil)
			diags.Extend(vdiags...)

			environment[kvp.Key.(*ast.StringExpr).Value] = v
		}
	}

	var parts []model.Expression
	for _, part := range node.Parts {
		parts = append(parts, plainLit(part.Text))

		if part.Value != nil {
			ref, rdiags := imp.importPropertyAccess(node, part.Value, environment, nil)
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

// importJoin imports a call to fn::join as a call to the `join` function.
func (imp *importer) importJoin(node *ast.JoinExpr) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	delim, ddiags := imp.importExpr(node.Delimiter, nil)
	diags.Extend(ddiags...)

	values, vdiags := imp.importExpr(node.Values, nil)
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

// importSplit imports a call to fn::split as a call to the `split` function.
func (imp *importer) importSplit(node *ast.SplitExpr) (model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	delim, ddiags := imp.importExpr(node.Delimiter, nil)
	diags.Extend(ddiags...)

	source, sdiags := imp.importExpr(node.Source, nil)
	diags.Extend(sdiags...)

	if diags.HasErrors() {
		return nil, diags
	}

	call := &model.FunctionCallExpression{
		Name: "split",
		Args: []model.Expression{delim, source},
	}
	return call, diags
}

// importFunctionCall imports a call to an AWS intrinsic function. The way the function is imported depends on the
// function:
//
// - `fn::asset` is imported as a call to the `fileAsset` function
// - `fn::invoke` is imported as a call to `invoke`
// - `fn::join` is imported as either a template expression or a call to `join`
// - `fn::split` is imported as a call to `split`
// - `fn::stackReference` is imported as a reference to the named stack
func (imp *importer) importBuiltin(node ast.BuiltinExpr) (model.Expression, syntax.Diagnostics) {
	switch node := node.(type) {
	case *ast.StringAssetExpr:
		path, pdiags := imp.importExpr(node.Source, nil)
		return &model.FunctionCallExpression{
			Name: "stringAsset",
			Args: []model.Expression{path},
		}, pdiags

	case *ast.FileAssetExpr:
		path, pdiags := imp.importExpr(node.Source, nil)
		return &model.FunctionCallExpression{
			Name: "fileAsset",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.RemoteAssetExpr:
		path, pdiags := imp.importExpr(node.Source, nil)
		return &model.FunctionCallExpression{
			Name: "remoteAsset",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.FileArchiveExpr:
		path, pdiags := imp.importExpr(node.Source, nil)
		return &model.FunctionCallExpression{
			Name: "fileArchive",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.RemoteArchiveExpr:
		path, pdiags := imp.importExpr(node.Source, nil)
		return &model.FunctionCallExpression{
			Name: "remoteArchive",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.AssetArchiveExpr:
		path, pdiags := imp.importExpr(node.Args(), nil)
		return &model.FunctionCallExpression{
			Name: "assetArchive",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.SecretExpr:
		path, pdiags := imp.importExpr(node.Args(), nil)
		return &model.FunctionCallExpression{
			Name: "secret",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.InvokeExpr:
		var diags syntax.Diagnostics

		version, err := pulumiyaml.ParseVersion(node.CallOpts.Version)
		if err != nil {
			return nil, syntax.Diagnostics{ast.ExprError(node.CallOpts.Version, fmt.Sprintf("unable to parse function provider version: %v", err), "")}
		}
		pkg, functionName, err := pulumiyaml.ResolveFunction(imp.loader, node.Token.Value, version)
		if err != nil {
			return nil, syntax.Diagnostics{ast.ExprError(node.Token, fmt.Sprintf("unable to resolve function name: %v", err), "")}
		}
		function := quotedLit(string(functionName))

		invokeArgs := []model.Expression{function}
		if node.CallArgs != nil {
			inputs := pkg.FunctionTypeHint(functionName).Inputs
			args, adiags := imp.importExpr(node.CallArgs, inputs)
			diags.Extend(adiags...)

			invokeArgs = append(invokeArgs, args)
		} else {
			invokeArgs = append(invokeArgs, &model.ObjectConsExpression{})
		}

		fn := &model.FunctionCallExpression{
			Name: "invoke",
			Args: invokeArgs,
		}
		if node.Return == nil {
			return fn, diags
		}
		return relativeTraversal(fn, node.Return.Value), diags
	case *ast.JoinExpr:
		return imp.importJoin(node)
	case *ast.SelectExpr:
		var diags syntax.Diagnostics

		index, idiags := imp.importExpr(node.Index, nil)
		diags.Extend(idiags...)

		values, vdiags := imp.importExpr(node.Values, nil)
		diags.Extend(vdiags...)

		return &model.IndexExpression{
			Collection: values,
			Key:        index,
		}, diags
	case *ast.SplitExpr:
		return imp.importSplit(node)
	case *ast.StackReferenceExpr:
		stackName := node.StackName.Value
		stackVar := imp.stackReferences[stackName]

		propertyName, diags := imp.importExpr(node.PropertyName, nil)

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
	case *ast.ReadFileExpr:
		var diags syntax.Diagnostics

		path, pdiags := imp.importExpr(node.Path, nil)
		diags.Extend(pdiags...)

		return &model.FunctionCallExpression{
			Name: "readFile",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.ToJSONExpr:
		path, pdiags := imp.importExpr(node.Args(), nil)
		return &model.FunctionCallExpression{
			Name: "toJSON",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.ToBase64Expr:
		path, pdiags := imp.importExpr(node.Args(), nil)
		return &model.FunctionCallExpression{
			Name: "toBase64",
			Args: []model.Expression{path},
		}, pdiags
	case *ast.FromBase64Expr:
		path, pdiags := imp.importExpr(node.Args(), nil)
		return &model.FunctionCallExpression{
			Name: "fromBase64",
			Args: []model.Expression{path},
		}, pdiags
	default:
		contract.Failf("unexpected builtin type %T", node)
		return nil, nil
	}
}

// importExpr imports an AST expression as its equivalent PCL. Most nodes are imported as one
// would expect (e.g. sequences -> tuple construction, maps -> object construction, etc.).
// Function calls are the lone exception; see importFunction for more details.
//
// Because yaml does not distinguish between maps (string to value) and objects (known keys and
// value heterogeneous values), we also pass a type hint where possible.
func (imp *importer) importExpr(node ast.Expr, hint schema.Type) (model.Expression, syntax.Diagnostics) {
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
			var eHint schema.Type
			if l, ok := codegen.UnwrapType(hint).(*schema.ArrayType); ok {
				eHint = l.ElementType
			}
			e, ediags := imp.importExpr(v, eHint)
			diags.Extend(ediags...)

			expressions = append(expressions, e)
		}
		return &model.TupleConsExpression{
			Expressions: expressions,
		}, diags
	case *ast.ObjectExpr:
		var diags syntax.Diagnostics
		var items []model.ObjectConsItem
		var fieldHints map[string]schema.Type
		if obj, ok := codegen.UnwrapType(hint).(*schema.ObjectType); ok && obj != nil {
			fieldHints = map[string]schema.Type{}
			for _, prop := range obj.Properties {
				fieldHints[prop.Name] = prop.Type
			}
		}
		for _, kvp := range node.Entries {
			var k model.Expression
			var hint schema.Type
			// We have a type hint, so this will be a plain string, not a quoted string
			// (template expression).
			if fieldHints != nil {
				prop := kvp.Key.(*ast.StringExpr).Value
				hint = fieldHints[prop]
				k = plainLit(prop)
			} else {
				var kdiags syntax.Diagnostics
				k, kdiags = imp.importExpr(kvp.Key, nil)
				diags.Extend(kdiags...)
			}

			v, vdiags := imp.importExpr(kvp.Value, hint)
			diags.Extend(vdiags...)

			items = append(items, model.ObjectConsItem{Key: k, Value: v})
		}
		return &model.ObjectConsExpression{
			Items: items,
		}, diags
	case *ast.InterpolateExpr:
		return imp.importInterpolate(node, nil)
	case *ast.SymbolExpr:
		if f, ok, diags := imp.pulumiPropertyAccess(node, node.Property.Accessors); ok {
			return f, diags
		}
		return imp.importPropertyAccess(node, node.Property, nil, hint)
	case ast.BuiltinExpr:
		return imp.importBuiltin(node)
	default:
		contract.Failf("unexpected YAML node of type %T", node)
		return nil, nil
	}
}

// importParameterType converts a YAML config variable type to its equivalent PCL type.
func importParameterType(s string) (string, bool) {
	t, ok := config.Parse(s)
	if !ok {
		return "", false
	}
	return t.Pcl().String(), true
}

// importConfig imports a template config variable. The parameter is imported as a simple config variable definition.
func (imp *importer) importConfig(kvp ast.ConfigMapEntry) (model.BodyItem, syntax.Diagnostics) {
	name, config := kvp.Key.Value, kvp.Value

	var typeExpr string
	if config.Type != nil {
		var ok bool
		typeExpr, ok = importParameterType(config.Type.Value)
		if !ok {
			return nil, syntax.Diagnostics{ast.ExprError(config.Type, fmt.Sprintf("unrecognized type '%v' for config variable '%s'", config.Type.Value, name), "")}
		}
	} else {
		typeExpr = "string"
	}

	configName, ok := imp.configuration[name]
	contract.Assertf(ok, "key %q not found in configuration map", name)

	var defaultValue model.Expression
	if config.Default != nil {
		v, diags := imp.importExpr(config.Default, nil)
		if diags.HasErrors() {
			return nil, diags
		}
		defaultValue = v
	}

	// TODO(pdg): secret configuration -- requires changes in PCL

	configDef := &model.Block{
		Type:   "config",
		Labels: []string{configName.Name, typeExpr},
		Body: &model.Body{
			Items: []model.BodyItem{
				&model.Attribute{
					Name:  pcl.LogicalNamePropertyKey,
					Value: quotedLit(kvp.Key.GetValue()),
				},
			},
		},
	}
	if defaultValue != nil {
		configDef.Body.Items = append(configDef.Body.Items, &model.Attribute{
			Name:  "default",
			Value: defaultValue,
		})
	}
	return configDef, nil
}

func (imp *importer) getResourceRefList(optionField ast.Expr, name string, field string) ([]model.Expression, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	elems, ok := optionField.(*ast.ListExpr)
	if !ok {
		diags.Extend(ast.ExprError(optionField, fmt.Sprintf("expected %v of resource '%v' to be a list of resource expressions, got '%v'", field, name, reflect.TypeOf(elems)), ""))
	}
	var refs []model.Expression
	for _, e := range elems.Elements {
		sym, ok := e.(*ast.SymbolExpr)
		if !ok {
			diags.Extend(ast.ExprError(optionField, fmt.Sprintf("expected %v of resource '%v' to be a list of resource expressions, got '%v'", field, name, reflect.TypeOf(e)), ""))
			continue
		}
		resourceName := sym.Property.Accessors[0].(*ast.PropertyName).Name
		if resourceVar, ok := imp.resources[resourceName]; ok {
			refs = append(refs, model.VariableReference(resourceVar))
		} else {
			diags.Extend(ast.ExprError(e, fmt.Sprintf("unknown resource '%v'", resourceName), ""))
		}
	}

	return refs, diags
}

func (imp *importer) getResourceRefItem(optionField ast.Expr, name string, field string) (model.Expression, *syntax.Diagnostic) {
	sym, ok := optionField.(*ast.SymbolExpr)
	if !ok {
		return nil, ast.ExprError(optionField, fmt.Sprintf("expected %v of resource '%v' to be a resource, got '%v'", field, name, reflect.TypeOf(sym)), "")
	}
	resourceName := sym.Property.Accessors[0].(*ast.PropertyName).Name
	resourceVar, ok := imp.resources[resourceName]
	if !ok {
		return nil, ast.ExprError(optionField, fmt.Sprintf("unknown resource '%v'", resourceName), "")
	}
	scoped := model.VariableReference(resourceVar)

	if len(sym.Property.Accessors) > 1 {
		// This is a complex expression, so we cannot verify at compile time
		// that it results in a resource at run time.
		//
		// TODO: Once we have a format for type checking, we could add a check
		// here when type information is present. We will never be able to rely
		// on type information, since we want to be able to eject out of YAML as
		// easily as possible.
		for _, prop := range sym.Property.Accessors[1:] {
			switch prop := prop.(type) {
			case *ast.PropertyName:
				scoped.Parts = append(scoped.Parts, model.StringType)
				scoped.Traversal = append(scoped.Traversal, hcl.TraverseAttr{Name: prop.Name})
			case *ast.PropertySubscript:
				switch i := prop.Index.(type) {
				case int:
					scoped.Parts = append(scoped.Parts, model.IntType)
					scoped.Traversal = append(scoped.Traversal, hcl.TraverseIndex{Key: cty.NumberIntVal(int64(i))})
				case string:
					scoped.Parts = append(scoped.Parts, model.StringType)
					scoped.Traversal = append(scoped.Traversal, hcl.TraverseAttr{Name: i})
				default:
					return nil, ast.ExprError(sym, fmt.Sprintf("unknown access type: %T", i), "")
				}
			default:
				return nil, ast.ExprError(sym, fmt.Sprintf("unknown property component: '%[1]s' of type %[1]T", prop), "")
			}
		}
	}

	return scoped, nil
}

// importVariable imports a YAML variable as a PCL variable.
func (imp *importer) importVariable(kvp ast.VariablesMapEntry, latestPkgInfo map[string]*packageInfo) (model.BodyItem, syntax.Diagnostics) {
	var diags syntax.Diagnostics
	name, value := kvp.Key.Value, kvp.Value
	_, ok := imp.variables[name]
	contract.Assertf(ok, "variable %q not found", name)

	v, vdiags := imp.importExpr(value, nil)
	diags.Extend(vdiags...)
	if vdiags.HasErrors() {
		return nil, diags
	}

	// set latest package info
	if v, ok := value.(*ast.InvokeExpr); ok {
		pkgName := strings.Split(v.Token.Value, ":")[0]
		if p, ok := latestPkgInfo[pkgName]; ok {
			if p.version != "" {
				v.CallOpts.Version = ast.String(p.version)
			}
			if p.pluginDownloadURL != "" {
				v.CallOpts.PluginDownloadURL = ast.String(p.pluginDownloadURL)
			}
		}
	}
	return &model.Attribute{
		Name:  imp.variables[name].Name,
		Value: v,
	}, diags
}

// gets the latest package version specified on a resource
func (imp *importer) getLatestPkgInfoResource(kvp ast.ResourcesMapEntry, pkgInfo map[string]*packageInfo) syntax.Diagnostics {
	resource := kvp.Value
	pkg := pulumiyaml.ResolvePkgName(resource.Type.Value)

	if resource.Options.Version != nil {
		url := ""
		if resource.Options.PluginDownloadURL != nil {
			url = resource.Options.PluginDownloadURL.Value
		}
		v1, err := pulumiyaml.ParseVersion(resource.Options.Version)
		if err == nil && v1 != nil {
			if p, ok := pkgInfo[pkg]; ok {
				v2, _ := semver.Make(p.version)
				if v1.Compare(v2) == 1 {
					p.version = v1.String()
					p.pluginDownloadURL = url
				}
			} else {
				pkgInfo[pkg] = &packageInfo{
					version:           v1.String(),
					pluginDownloadURL: url,
				}
			}
		}
	}
	return nil
}

// gets the latest package version specified on a variable (for invokes)
func (imp *importer) getLatestPkgInfoVariable(kvp ast.VariablesMapEntry, pkgInfo map[string]*packageInfo) {
	value := kvp.Value
	if v, ok := value.(*ast.InvokeExpr); ok {
		if v.CallOpts.Version == nil {
			return
		}

		pkgName := strings.Split(v.Token.Value, ":")[0]
		url := ""
		if v.CallOpts.PluginDownloadURL != nil {
			url = v.CallOpts.PluginDownloadURL.Value
		}
		v1, err := pulumiyaml.ParseVersion(v.CallOpts.Version)
		if err == nil && v1 != nil {
			if p, ok := pkgInfo[pkgName]; ok {
				v2, _ := semver.Make(p.version)
				if v1.Compare(v2) == 1 {
					p.version = v1.String()
					p.pluginDownloadURL = url
				}
			} else {
				pkgInfo[pkgName] = &packageInfo{
					version:           v1.String(),
					pluginDownloadURL: url,
				}
			}
		}
	}
}

// importResource imports a YAML resource as a PCL resource.
func (imp *importer) importResource(kvp ast.ResourcesMapEntry, latestPkgInfo map[string]*packageInfo) (model.BodyItem, syntax.Diagnostics) {
	name, resource := kvp.Key.Value, kvp.Value

	resourceVar, ok := imp.resources[name]
	contract.Assertf(ok, "resource %q not found", name)

	var diags syntax.Diagnostics

	version, err := pulumiyaml.ParseVersion(resource.Options.Version)
	if err != nil {
		diags.Extend(ast.ExprError(resource.Options.Version, fmt.Sprintf("unable to parse resource %v provider version: %v", name, err), ""))
		return nil, diags
	}
	pkg, token, err := pulumiyaml.ResolveResource(imp.loader, resource.Type.Value, version)
	if err != nil {
		return nil, syntax.Diagnostics{ast.ExprError(resource.Type, fmt.Sprintf("unable to resolve resource type: %v", err), "")}
	}
	props := pkg.ResourceTypeHint(token)
	contract.Assertf(props != nil,
		"token(%s) was obtained by the same ResolveResource call as pkg(%s),"+
			" so must produce a non nil value", token.String(), pkg.Name())
	items := []model.BodyItem{
		&model.Attribute{
			Name:  pcl.LogicalNamePropertyKey,
			Value: quotedLit(name),
		},
	}

	hints := map[string]schema.Type{}
	if props.Resource != nil {
		for _, input := range props.Resource.InputProperties {
			hints[input.Name] = input.Type
		}
	}
	for _, kvp := range resource.Properties.Entries {
		v, vdiags := imp.importExpr(kvp.Value, hints[kvp.Key.Value])
		diags.Extend(vdiags...)
		items = append(items, &model.Attribute{
			Name:  kvp.Key.Value,
			Value: v,
		})
	}

	// TODO: resource options not supported by PCL: component, additional secret outputs, aliases, custom timeouts, delete before replace, import, version

	resourceOptions := &model.Block{
		Type: "options",
		Body: &model.Body{},
	}
	if resource.Options.DependsOn != nil {
		refs, rdiags := imp.getResourceRefList(resource.Options.DependsOn, name, "dependsOn")
		diags.Extend(rdiags...)
		if len(refs) > 0 {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name: "dependsOn",
				Value: &model.TupleConsExpression{
					Expressions: refs,
				},
			})
		}
	}
	if len(resource.Options.IgnoreChanges.GetElements()) != 0 {
		var paths []model.Expression
		for _, v := range resource.Options.IgnoreChanges.Elements {
			paths = append(paths, plainLit(v.Value))
		}
		resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
			Name:  "ignoreChanges",
			Value: &model.TupleConsExpression{Expressions: paths},
		})
	}
	if resource.Options.Parent != nil {
		ref, err := imp.getResourceRefItem(resource.Options.Parent, name, "parent")
		if err != nil {
			diags.Extend(err)
		} else {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name:  "parent",
				Value: ref,
			})
		}
	}
	if resource.Options.Protect != nil {
		protectExpr, vdiags := imp.importExpr(resource.Options.Protect, schema.BoolType)
		diags.Extend(vdiags...)
		resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
			Name:  "protect",
			Value: protectExpr,
		})
	}
	// latest package settings takes precedence over a set provider/ version/ url
	if p, ok := latestPkgInfo[pkg.Name()]; ok {
		if p.version != "" {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name:  "version",
				Value: quotedLit(p.version),
			})
		}
		if p.pluginDownloadURL != "" {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name:  "pluginDownloadURL",
				Value: quotedLit(p.pluginDownloadURL),
			})
		}
	} else {
		if resource.Options.Provider != nil {
			ref, err := imp.getResourceRefItem(resource.Options.Provider, name, "provider")
			if err != nil {
				diags.Extend(err)
			} else {
				resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
					Name:  "provider",
					Value: ref,
				})
			}
		}
		if resource.Options.Version != nil {
			parsedVersion, err := pulumiyaml.ParseVersion(resource.Options.Version)
			if err == nil && parsedVersion != nil {
				resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
					Name:  "version",
					Value: quotedLit(parsedVersion.String()),
				})
			}
		}
		if resource.Options.PluginDownloadURL != nil {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name:  "pluginDownloadURL",
				Value: quotedLit(resource.Options.PluginDownloadURL.Value),
			})
		}
	}

	if resource.Options.Providers != nil {
		refs, rdiags := imp.getResourceRefList(resource.Options.Providers, name, "providers")
		diags.Extend(rdiags...)
		if len(refs) > 0 {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name: "providers",
				Value: &model.TupleConsExpression{
					Expressions: refs,
				},
			})
		}
	}

	if resource.Options.DeletedWith != nil {
		ref, err := imp.getResourceRefItem(resource.Options.DeletedWith, name, "deletedWith")
		if err != nil {
			diags.Extend(err)
		} else {
			resourceOptions.Body.Items = append(resourceOptions.Body.Items, &model.Attribute{
				Name:  "deletedWith",
				Value: ref,
			})
		}
	}

	if len(resourceOptions.Body.Items) > 0 {
		items = append(items, resourceOptions)
	}

	r := &model.Block{
		Type:   "resource",
		Labels: []string{resourceVar.Name, string(token)},
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
	contract.Assertf(ok, "output %q not found", name)

	x, diags := imp.importExpr(kvp.Value, nil)

	return &model.Block{
		Type:   "output",
		Labels: []string{outputVar.Name},
		Body: &model.Body{
			Items: []model.BodyItem{
				&model.Attribute{
					Name:  pcl.LogicalNamePropertyKey,
					Value: quotedLit(name),
				},
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
	// PCL has only one namspace with respect to binding, so we can't use any of
	// these as names.
	assigned := codegen.NewStringSet(
		"element",
		"entries",
		"fileArchive",
		"fileAsset",
		"filebase64",
		"filebase64sha256",
		"fromBase64",
		"invoke",
		"join",
		"length",
		"lookup",
		"range",
		"readFile",
		"readDir",
		"secret",
		"split",
		"toBase64",
		"toJSON",
		"sha1",
		"stack",
		"project",
		"cwd",
	)

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

	for _, kvp := range append(file.Configuration.Entries, file.Config.Entries...) {
		imp.configuration[kvp.Key.Value] = nil
	}
	for _, kvp := range file.Resources.Entries {
		for _, kvp := range kvp.Value.Properties.Entries {
			imp.findStackReferences(kvp.Value)
		}
		imp.resources[kvp.Key.Value] = nil
	}
	for _, kvp := range file.Variables.Entries {
		imp.variables[kvp.Key.Value] = nil
	}
	for _, kvp := range file.Outputs.Entries {
		imp.findStackReferences(kvp.Value)
		imp.outputs[kvp.Key.Value] = nil
	}
	imp.assignNames()

	var items []model.BodyItem

	// Import config.
	for _, kvp := range append(file.Configuration.Entries, file.Config.Entries...) {
		config, cdiags := imp.importConfig(kvp)
		diags.Extend(cdiags...)

		if config != nil {
			items = append(items, config)
		}
	}

	// get latest package info
	latestPkgInfo := make(map[string]*packageInfo)
	for _, kvp := range file.Resources.Entries {
		rdiags := imp.getLatestPkgInfoResource(kvp, latestPkgInfo)
		diags.Extend(rdiags...)
	}
	for _, kvp := range file.Variables.Entries {
		imp.getLatestPkgInfoVariable(kvp, latestPkgInfo)
	}

	// Import variables
	for _, kvp := range file.Variables.Entries {
		output, vdiags := imp.importVariable(kvp, latestPkgInfo)
		diags.Extend(vdiags...)

		if output != nil {
			items = append(items, output)
		}
	}

	// Import stack references.
	//
	// TODO: this isn't supported by PCL.
	for _, name := range imp.referencedStacks {
		stackVar, ok := imp.stackReferences[name]
		contract.Assertf(ok, "stack reference %q not found", name)

		items = append(items, &model.Block{
			Type:   "stackReference",
			Labels: []string{stackVar.Name, name},
			Body:   &model.Body{},
		})
	}

	// Import resources.
	for _, kvp := range file.Resources.Entries {
		resource, rdiags := imp.importResource(kvp, latestPkgInfo)
		diags.Extend(rdiags...)

		if resource != nil {
			items = append(items, resource)
		}
	}

	// Import outputs.
	for _, kvp := range file.Outputs.Entries {
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
func ImportTemplate(file *ast.TemplateDecl, loader pulumiyaml.PackageLoader) (*model.Body, syntax.Diagnostics) {

	imp := importer{
		loader:          loader,
		configuration:   map[string]*model.Variable{},
		variables:       map[string]*model.Variable{},
		stackReferences: map[string]*model.Variable{},
		resources:       map[string]*model.Variable{},
		outputs:         map[string]*model.Variable{},
	}
	return imp.importTemplate(file)
}
