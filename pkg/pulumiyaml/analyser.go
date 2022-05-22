// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

type typeCache struct {
	exprs         map[ast.Expr]schema.Type
	resources     map[*ast.ResourceDecl]schema.Type
	resourceNames map[string]*ast.ResourceDecl
	variableNames map[string]ast.Expr
}

func (tc *typeCache) registerResource(name string, resource *ast.ResourceDecl, typ schema.Type) {
	tc.resourceNames[name] = resource
	tc.resources[resource] = typ
}

func isAssignable(from, to schema.Type) bool {
	to = codegen.UnwrapType(to)
	from = codegen.UnwrapType(from)
	if schema.IsPrimitiveType(to) {
		switch to {
		case schema.NumberType, schema.IntType:
			return from == schema.NumberType || from == schema.IntType
		case schema.AnyType:
			return true
		default:
			return from == to
		}
	}

	switch from := from.(type) {
	case *schema.ArrayType:
		to, ok := to.(*schema.ArrayType)
		if !ok {
			return false
		}
		return isAssignable(from.ElementType, to.ElementType)
	case *schema.MapType:
		to, ok := to.(*schema.MapType)
		if !ok {
			return false
		}
		return isAssignable(from.ElementType, to.ElementType)
	}

	// TODO
	return false
}

func assertTypeAssignable(ctx *evalContext, loc *hcl.Range, from, to schema.Type) {
	if isAssignable(from, to) {
		return
	}
	ctx.addDiag(syntax.Error(loc, fmt.Sprintf("%s is not assignable from %s", to, from), " "))
}

func (tc *typeCache) typeResource(r *runner, node resourceNode) bool {
	k, v := node.Key.Value, node.Value
	ctx := r.newContext(node)
	pkg, typ, err := ResolveResource(ctx.pkgLoader, v.Type.Value)
	if err != nil {
		ctx.error(v.Type, fmt.Sprintf("error resolving type of resource %v: %v", k, err))
		return true
	}
	hint := pkg.ResourceTypeHint(typ)
	properties := map[string]*schema.Property{}
	for _, prop := range hint.Resource.InputProperties {
		properties[prop.Name] = prop
	}
	var allProperties []string
	for k := range properties {
		allProperties = append(allProperties, k)
	}
	fmtr := yamldiags.NonExistantFieldFormatter{
		ParentLabel:         fmt.Sprintf("Resource %s", typ.String()),
		Fields:              allProperties,
		MaxElements:         5,
		FieldsAreProperties: true,
	}

	for _, kvp := range v.Properties.Entries {
		if typ, hasField := properties[kvp.Key.Value]; !hasField {
			summary, detail := fmtr.MessageWithDetail(kvp.Key.Value, fmt.Sprintf("Property %s", kvp.Key.Value))
			subject := kvp.Key.Syntax().Syntax().Range()
			valueRange := kvp.Value.Syntax().Syntax().Range()
			context := hcl.RangeOver(*subject, *valueRange)
			ctx.addDiag(syntax.Error(subject, summary, detail).WithContext(&context))
		} else {
			existing, ok := tc.exprs[kvp.Value]
			rng := kvp.Key.Syntax().Syntax().Range()
			if !ok {
				ctx.addDiag(syntax.Warning(rng,
					fmt.Sprintf("internal error: untyped input for %s.%s", k, kvp.Key.Value),
					fmt.Sprintf("expected type %s", typ.Type)))
			} else {
				assertTypeAssignable(ctx, rng, existing, typ.Type)
			}
		}
	}
	tc.registerResource(k, node.Value, hint)

	// TODO: type check resource options

	// Check for extra fields that didn't make it into the resource or resource options object
	options := ResourceOptionsTypeHint()
	allOptions := make([]string, 0, len(options))
	for k := range options {
		allOptions = append(allOptions, k)
	}
	if s := v.Syntax(); s != nil {
		if o, ok := s.(*syntax.ObjectNode); ok {
			validKeys := []string{"type", "properties", "options", "condition", "metadata"}
			fmtr := yamldiags.InvalidFieldBagFormatter{
				ParentLabel: fmt.Sprintf("Resource %s", typ.String()),
				MaxListed:   5,
				Bags: []yamldiags.TypeBag{
					{Name: "properties", Properties: allProperties},
					{Name: "options", Properties: allOptions},
					{Name: k, Properties: validKeys},
				},
				DistanceLimit: 3,
			}
			for i := 0; i < o.Len(); i++ {
				prop := o.Index(i)
				key := prop.Key.Value()
				keyLower := strings.ToLower(key)
				valid := false
				for _, k := range validKeys {
					if k == keyLower {
						valid = true
						break
					}
				}
				if valid {
					// This is a valid option, so we don't error
					continue
				}

				summary, detail := fmtr.MessageWithDetail(key)
				if match := fmtr.ExactMatching(key); len(match) == 1 {
					detail += fmt.Sprintf(", e.g.\n\n%s:\n  # ...\n  %s:\n    %s: %s",
						k, match[0], key, prop.Value)
				}

				subject := prop.Key.Syntax().Range()
				ctx.addDiag(syntax.Error(subject, summary, detail))
			}
		}
	}

	if s := v.Options.Syntax(); s != nil {
		if o, ok := s.(*syntax.ObjectNode); ok {
			fmtr := yamldiags.NonExistantFieldFormatter{
				ParentLabel:         "resource options",
				Fields:              allOptions,
				MaxElements:         5,
				FieldsAreProperties: false,
			}
			optionsLower := map[string]struct{}{}
			for k := range options {
				optionsLower[strings.ToLower(k)] = struct{}{}
			}
			for i := 0; i < o.Len(); i++ {
				prop := o.Index(i)
				key := prop.Key.Value()
				keyLower := strings.ToLower(key)
				if _, has := optionsLower[keyLower]; has {
					continue
				}
				summary, detail := fmtr.MessageWithDetail(key, key)
				subject := prop.Key.Syntax().Range()
				ctx.addDiag(syntax.Error(subject, summary, detail))
			}
		}
	}

	return true
}

func (tc *typeCache) typeInvoke(ctx *evalContext, t *ast.InvokeExpr) bool {
	pkg, functionName, err := ResolveFunction(ctx.pkgLoader, t.Token.Value)
	if err != nil {
		_, b := ctx.error(t, err.Error())
		return b
	}
	var existing []string
	hint := pkg.FunctionTypeHint(functionName)
	inputs := map[string]schema.Type{}
	if hint.Inputs != nil {
		for _, input := range hint.Inputs.Properties {
			existing = append(existing, input.Name)
			inputs[input.Name] = input.Type
		}
	}
	fmtr := yamldiags.NonExistantFieldFormatter{
		ParentLabel: fmt.Sprintf("Invoke %s", functionName.String()),
		Fields:      existing,
		MaxElements: 5,
	}
	if t.CallArgs != nil {
		for _, prop := range t.CallArgs.Entries {
			k := prop.Key.(*ast.StringExpr).Value
			if typ, ok := inputs[k]; !ok {
				summary, detail := fmtr.MessageWithDetail(k, k)
				subject := prop.Key.Syntax().Syntax().Range()
				context := t.Syntax().Syntax().Range()
				ctx.addDiag(syntax.Error(subject, summary, detail).WithContext(context))
			} else {
				tc.exprs[prop.Value] = typ
			}
		}
	}
	if t.Return != nil {
		fields := []string{}
		var (
			returnType  schema.Type
			validReturn bool
		)
		if o := hint.Outputs; o != nil {
			for _, output := range o.Properties {
				fields = append(fields, output.Name)
				if strings.ToLower(t.Return.Value) == strings.ToLower(output.Name) {
					returnType = output.Type
					validReturn = true
				}
			}
		}
		fmtr := yamldiags.NonExistantFieldFormatter{
			ParentLabel:         t.Token.Value,
			Fields:              fields,
			MaxElements:         5,
			FieldsAreProperties: true,
		}
		if hint.Outputs == nil || !validReturn {
			summary, detail := fmtr.MessageWithDetail(t.Return.Value, t.Return.Value)
			ctx.addDiag(syntax.Error(t.Return.Syntax().Syntax().Range(), summary, detail))
		} else {
			tc.exprs[t] = returnType
		}
	} else {
		tc.exprs[t] = hint.Outputs
	}
	return true
}

func (tc *typeCache) typeSymbol(ctx *evalContext, t *ast.SymbolExpr) bool {
	var typ schema.Type = &schema.InvalidType{}
	setType := func(t schema.Type) { typ = t }
	setError := func(diag *syntax.Diagnostic) {
		ctx.addDiag(diag)
		typ = &schema.InvalidType{
			Diagnostics: []*hcl.Diagnostic{diag.HCL()},
		}
	}
	if root, ok := tc.resourceNames[t.Property.RootName()]; ok {
		typ = tc.resources[root]
	}
	if root, ok := tc.variableNames[t.Property.RootName()]; ok {
		typ = tc.exprs[root]
	}
	// TODO handle config here
	runningName := t.Property.RootName()
	for _, accessor := range t.Property.Accessors[1:] {
		switch accessor := accessor.(type) {
		case *ast.PropertyName:
			properties := map[string]schema.Type{}
			switch typ := codegen.UnwrapType(typ).(type) {
			case *schema.ObjectType:
				for _, prop := range typ.Properties {
					properties[prop.Name] = prop.Type
				}
			case *schema.ResourceType:
				for _, prop := range typ.Resource.Properties {
					properties[prop.Name] = prop.Type
				}
			case *schema.InvalidType:
				break
			default:
				setError(syntax.Error(t.Syntax().Syntax().Range(),
					fmt.Sprintf("cannot access a property on '%s' (type %s)", runningName, codegen.UnwrapType(typ)),
					"Property access is only allowed on Resources and Objects"))
				break
			}
			// We handle the actual property access here
			newType, ok := properties[accessor.Name]
			if !ok {
				propertyList := make([]string, 0, len(properties))
				for k := range properties {
					propertyList = append(propertyList, k)
				}
				fmtr := yamldiags.NonExistantFieldFormatter{
					ParentLabel:         runningName,
					Fields:              propertyList,
					MaxElements:         5,
					FieldsAreProperties: true,
				}
				summary, detail := fmtr.MessageWithDetail(accessor.Name, accessor.Name)
				setError(syntax.Error(t.Syntax().Syntax().Range(), summary, detail))
				break
			}
			runningName += "." + accessor.Name
			setType(newType)
		case *ast.PropertySubscript:
			err := func(msg string) {
				setError(syntax.Error(t.Syntax().Syntax().Range(),
					fmt.Sprintf("cannot index into '%s' (type %s)", runningName, codegen.UnwrapType(typ)), msg))
			}
			switch typ := codegen.UnwrapType(typ).(type) {
			case *schema.ArrayType:
				if _, ok := accessor.Index.(string); ok {
					err("Index via string is only allowed on Maps")
				}
				runningName += fmt.Sprintf("[%d]", accessor.Index.(int))
				setType(typ.ElementType)
			case *schema.MapType:
				if _, ok := accessor.Index.(int); ok {
					err("Index via number is only allowed on Arrays")
				}
				runningName += fmt.Sprintf("[%q]", accessor.Index.(string))
				setType(typ.ElementType)
			case *schema.InvalidType:
				break
			default:
				err("Index property access is only allowed on Maps and Lists")
				break
			}
		default:
			panic(fmt.Sprintf("Unknown property type: %T", accessor))
		}
	}
	tc.exprs[t] = typ
	return true
}

func (tc *typeCache) typeExpr(ctx *evalContext, t ast.Expr) bool {
	switch t := t.(type) {
	case *ast.InvokeExpr:
		return tc.typeInvoke(ctx, t)
	case *ast.SymbolExpr:
		return tc.typeSymbol(ctx, t)
	case *ast.StringExpr:
		tc.exprs[t] = schema.StringType
	case *ast.NumberExpr:
		tc.exprs[t] = schema.NumberType
	case *ast.BooleanExpr:
		tc.exprs[t] = schema.BoolType
	case *ast.AssetArchiveExpr, *ast.FileArchiveExpr, *ast.RemoteArchiveExpr:
		tc.exprs[t] = schema.ArchiveType
	case *ast.FileAssetExpr, *ast.RemoteAssetExpr, *ast.StringAssetExpr:
		tc.exprs[t] = schema.AssetType
	case *ast.InterpolateExpr:
		// TODO: verify that internal access can be coerced into a string
		tc.exprs[t] = schema.StringType
	case *ast.ToJSONExpr:
		tc.exprs[t] = schema.StringType
	case *ast.JoinExpr:
		assertTypeAssignable(ctx, t.Delimiter.Syntax().Syntax().Range(), tc.exprs[t.Delimiter], schema.StringType)
		tc.exprs[t] = schema.StringType
	case *ast.ListExpr:
		// TODO: make this a union type
		tc.exprs[t] = &schema.ArrayType{
			ElementType: schema.AnyType,
		}
	case *ast.NullExpr:
		tc.exprs[t] = &schema.InvalidType{}
	}
	return true
}

func (tc *typeCache) typeVariable(r *runner, node variableNode) bool {
	k, v := node.Key.Value, node.Value
	tc.variableNames[k] = v
	return true
}

func newTypeCache() *typeCache {
	return &typeCache{
		exprs:         map[ast.Expr]schema.Type{},
		resources:     map[*ast.ResourceDecl]schema.Type{},
		resourceNames: map[string]*ast.ResourceDecl{},
		variableNames: map[string]ast.Expr{},
	}
}

func TypeCheck(r *runner) syntax.Diagnostics {
	types := newTypeCache()

	// Set roots
	diags := r.Run(walker{
		VisitResource: types.typeResource,
		VisitExpr:     types.typeExpr,
		VisitVariable: types.typeVariable,
	})

	return diags
}

type walker struct {
	VisitConfig   func(r *runner, node configNode) bool
	VisitVariable func(r *runner, node variableNode) bool
	VisitOutput   func(r *runner, node ast.PropertyMapEntry) bool
	VisitResource func(r *runner, node resourceNode) bool
	VisitExpr     func(*evalContext, ast.Expr) bool
}

func (e walker) walk(ctx *evalContext, x ast.Expr) bool {
	if x == nil {
		return true
	}
	switch x := x.(type) {
	case *ast.NullExpr, *ast.BooleanExpr, *ast.NumberExpr, *ast.StringExpr:
	case *ast.ListExpr:
		for _, el := range x.Elements {
			if !e.walk(ctx, el) {
				return false
			}
		}
	case *ast.ObjectExpr:
		for _, prop := range x.Entries {
			if !e.walk(ctx, prop.Key) {
				return false
			}
			if !e.walk(ctx, prop.Value) {
				return false
			}
		}
	case *ast.InterpolateExpr, *ast.SymbolExpr:
	case ast.BuiltinExpr:
		if !e.walk(ctx, x.Name()) {
			return false
		}
		if !e.walk(ctx, x.Args()) {
			return false
		}
	default:
		panic(fmt.Sprintf("fatal: invalid expr type %T", x))
	}
	if !e.VisitExpr(ctx, x) {
		return false
	}

	return true
}

func (e walker) EvalConfig(r *runner, node configNode) bool {
	if e.VisitConfig != nil {
		if !e.VisitConfig(r, node) {
			return false
		}
	}
	if e.VisitExpr != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		if !e.walk(ctx, node.Value.Default) {
			return false
		}
		if !e.walk(ctx, node.Value.Secret) {
			return false
		}
	}
	return true
}
func (e walker) EvalVariable(r *runner, node variableNode) bool {
	if e.VisitExpr != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		if !e.walk(ctx, node.Value) {
			return false
		}
	}
	if e.VisitVariable != nil {
		if !e.VisitVariable(r, node) {
			return false
		}
	}
	return true
}
func (e walker) EvalOutput(r *runner, node ast.PropertyMapEntry) bool {
	if e.VisitOutput != nil {
		if !e.VisitOutput(r, node) {
			return false
		}
	}
	if e.VisitExpr != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		if !e.walk(ctx, node.Value) {
			return false
		}
	}
	return true
}
func (e walker) EvalResource(r *runner, node resourceNode) bool {
	if e.VisitExpr != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		v := node.Value
		if !e.walk(ctx, v.Type) {
			return false
		}
		for _, prop := range v.Properties.Entries {
			if !e.walk(ctx, prop.Key) {
				return false
			}
			if !e.walk(ctx, prop.Value) {
				return false
			}
		}
		if !e.walkResourceOptions(ctx, v.Options) {
			return false
		}
	}
	// We visit the expressions in a resource before we visit the resource itself
	if e.VisitResource != nil {
		if !e.VisitResource(r, node) {
			return false
		}
	}
	return true
}

func (e walker) walkResourceOptions(ctx *evalContext, opts ast.ResourceOptionsDecl) bool {
	if !e.walkStringList(ctx, opts.AdditionalSecretOutputs) {
		return false
	}
	if !e.walkStringList(ctx, opts.Aliases) {
		return false
	}
	if !e.walk(ctx, opts.DeleteBeforeReplace) {
		return false
	}
	if !e.walk(ctx, opts.DependsOn) {
		return false
	}
	if !e.walkStringList(ctx, opts.IgnoreChanges) {
		return false
	}
	if !e.walk(ctx, opts.Import) {
		return false
	}
	if !e.walk(ctx, opts.Parent) {
		return false
	}
	if !e.walk(ctx, opts.Protect) {
		return false
	}
	if !e.walk(ctx, opts.Provider) {
		return false
	}
	if !e.walk(ctx, opts.Providers) {
		return false
	}
	if !e.walk(ctx, opts.Version) {
		return false
	}
	if !e.walk(ctx, opts.PluginDownloadURL) {
		return false
	}
	if !e.walkStringList(ctx, opts.ReplaceOnChanges) {
		return false
	}
	if !e.walk(ctx, opts.RetainOnDelete) {
		return false
	}

	if ct := opts.CustomTimeouts; ct != nil {
		if !e.walk(ctx, ct.Create) {
			return false
		}
		if !e.walk(ctx, ct.Delete) {
			return false
		}
		if !e.walk(ctx, ct.Update) {
			return false
		}
	}
	return true
}

func (e walker) walkStringList(ctx *evalContext, l *ast.StringListDecl) bool {
	if l != nil {
		for _, el := range l.Elements {
			if !e.walk(ctx, el) {
				return false
			}
		}
	}
	return true
}

// Compute the set of fields valid for the resource options.
func ResourceOptionsTypeHint() map[string]struct{} {
	typ := reflect.TypeOf(ast.ResourceOptionsDecl{})
	m := map[string]struct{}{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if name != "" {
			name = strings.ToLower(name[:1]) + name[1:]
		}
		m[name] = struct{}{}
	}
	return m
}
