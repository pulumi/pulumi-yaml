// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	ctypes "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/config"
	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

// Query the typing of a typed program.
//
// If the program failed to establish the type of a variable, then `*schema.InvalidType`
// is returned. If the the variable/expr is unknown to the typed program, `nil` is
// returned.
type Typing interface {
	TypeResource(name string) schema.Type
	TypeVariable(name string) schema.Type
	TypeConfig(name string) schema.Type
	TypeOutput(name string) schema.Type

	// TypeExpr can compare `ast.Expr` by pointer, so only expressions taken directly from
	// the program will return non-nil results.
	TypeExpr(expr ast.Expr) schema.Type
}

func (tc *typeCache) TypeResource(name string) schema.Type {
	decl, ok := tc.resourceNames[name]
	if !ok {
		return nil
	}
	return tc.resources[decl]
}

func (tc *typeCache) TypeVariable(name string) schema.Type {
	expr, ok := tc.variableNames[name]
	if !ok {
		return nil
	}
	return tc.exprs[expr]
}

func (tc *typeCache) TypeConfig(name string) schema.Type {
	return tc.configuration[name]
}

func (tc *typeCache) TypeOutput(name string) schema.Type {
	return tc.outputs[name]
}

func (tc *typeCache) TypeExpr(expr ast.Expr) schema.Type {
	return tc.exprs[expr]

}

type typeCache struct {
	resources     map[*ast.ResourceDecl]schema.Type
	configuration map[string]schema.Type
	outputs       map[string]schema.Type
	exprs         map[ast.Expr]schema.Type
	resourceNames map[string]*ast.ResourceDecl
	variableNames map[string]ast.Expr
}

func (tc *typeCache) registerResource(name string, resource *ast.ResourceDecl, typ schema.Type) {
	tc.resourceNames[name] = resource
	tc.resources[resource] = typ
}

// notAssignable represents the logic chain for why an assignment is not possible.
type notAssignable struct {
	reason   string
	because  []*notAssignable
	internal bool
	property string
}

func (n notAssignable) String() string {
	return n.string(0)
}

func (n notAssignable) string(indent int) string {
	var prop string
	if n.property != "" {
		prop = n.property + ": "
	}
	s := strings.Repeat("  ", indent) + prop + n.reason
	if len(n.because) > 0 {
		s += ":"
	}
	for _, arg := range n.because {
		s += "\n" + arg.string(indent+1)
	}
	return s
}

func (n notAssignable) Error() string {
	return strings.ReplaceAll(n.String(), "\n", ";")
}

func (n notAssignable) IsInternal() bool {
	if n.internal {
		return true
	}
	for _, b := range n.because {
		if b.IsInternal() {
			return true
		}
	}
	return false
}

func (n *notAssignable) Because(b ...*notAssignable) *notAssignable {
	if n == nil {
		return nil
	}
	c := *n
	c.because = b
	return &c
}

func (n *notAssignable) Property(propName string) *notAssignable {
	if n == nil {
		return nil
	}
	c := *n
	c.property = propName
	return &c
}

func (n *notAssignable) WithReason(reason string, a ...interface{}) *notAssignable {
	if n == nil {
		return nil
	}

	c := *n
	c.reason += fmt.Sprintf(reason, a...)
	return &c
}

const adhockObjectToken = "pulumi:adhock:" //nolint:gosec

func displayType(t schema.Type) string {
	return yamldiags.DisplayTypeWithAdhock(t, adhockObjectToken)
}

// isAssignable determines if the type `from` is assignable to the type `to`.
// If the assignment is legal, nil is returned.
func isAssignable(from, to schema.Type) *notAssignable {
	to = codegen.UnwrapType(to)
	from = codegen.UnwrapType(from)

	// If either type is invalid, we return. An error message should have
	// already been generated, we don't need to add another one.
	if _, ok := from.(*schema.InvalidType); ok {
		return nil
	}
	if _, ok := to.(*schema.InvalidType); ok {
		return nil
	}

	dispType := func(t schema.Type) string {
		var maybeType string
		if schema.IsPrimitiveType(from) {
			maybeType = "type "
		}
		return fmt.Sprintf("%s'%s'", maybeType, displayType(t))
	}

	fail := &notAssignable{
		reason: fmt.Sprintf("Cannot assign %s to %s",
			dispType(from), dispType(to)),
		internal: false,
	}
	okIf := func(cond bool) *notAssignable {
		if !cond {
			return fail
		}
		return nil
	}
	okIfAssignable := func(s *notAssignable) *notAssignable {
		if s == nil {
			return nil
		}
		return fail.Because(s)
	}

	// If from is a union type, then it can only assign if every element it contains can
	// be assigned to `to`.
	switch from := from.(type) {
	case *schema.UnionType:
		reasons := []*notAssignable{}
		for _, subtype := range from.ElementTypes {
			because := isAssignable(subtype, to)
			if because != nil {
				reasons = append(reasons, because)
			}
		}
		// Every assignment type must be assignable.
		return okIf(len(reasons) == 0).Because(reasons...)

	case *schema.TokenType:
		underlying := schema.AnyType
		if from.UnderlyingType != nil {
			underlying = from.UnderlyingType
		}
		check := isAssignable(underlying, to)
		return okIfAssignable(check).WithReason(". '%s' is a Token Type. Token types act like their underlying type", displayType(from))
	}

	if schema.IsPrimitiveType(to) {
		switch to {
		case schema.AnyType:
			return okIf(true)
		case schema.NumberType, schema.IntType:
			return okIf(from == schema.NumberType || from == schema.IntType)
		case schema.StringType:
			// Resources can coerce into strings (by implicitly calling urn)
			_, isResource := from.(*schema.ResourceType)
			return okIf(isResource || from == schema.StringType ||
				// Since we don't have a fn(number) -> string function, we coerce numbers to strings
				from == schema.NumberType ||
				from == schema.IntType ||
				from == schema.BoolType)
		case schema.AssetType:
			// Some schema fields with given type Asset actually accept either
			// Assets or Archives. We accept some invalid inputs instead of
			// rejecting valid inputs.
			return okIf(from == schema.AssetType || from == schema.ArchiveType)
		default:
			return okIf(from == to)
		}
	}

	switch to := to.(type) {
	case *schema.UnionType:
		reasons := []*notAssignable{}
		for _, subtype := range to.ElementTypes {
			because := isAssignable(from, subtype)
			if because == nil {
				return nil
			}
			reasons = append(reasons, because)
		}
		return fail.Because(reasons...)
	case *schema.ArrayType:
		from, ok := from.(*schema.ArrayType)
		if !ok {
			return fail
		}
		return okIfAssignable(isAssignable(from.ElementType, to.ElementType))
	case *schema.MapType:
		switch from := from.(type) {
		case *schema.MapType:
			return okIfAssignable(isAssignable(from.ElementType, to.ElementType))
		case *schema.ObjectType:
			// YAML does not distinguish between maps and objects, but our type system does.
			// We allow implicit conversions from YAML objects into maps.
			if len(from.Properties) == 0 {
				// The object has no properties, so we coerce it's type to the type of the
				// map trivially
				return okIf(true)
			}
			for _, prop := range from.Properties {
				notOk := isAssignable(prop.Type, to.ElementType)
				if notOk != nil {
					return fail.Because(notOk.Property(prop.Name))
				}
			}
			return okIf(true)
		default:
			return fail
		}
	case *schema.ResourceType:
		from, ok := from.(*schema.ResourceType)
		return okIf(ok && to.Token == from.Token)
	case *schema.EnumType:
		notAssignable := isAssignable(from, to.ElementType)
		if notAssignable != nil {
			return fail
		}
		// TODO: check that known enum values are type checked against valid
		// values e.g. string "Foo" should not be assignable to
		// type Enum { Type: string, Elements: ["fizz", "buzz"] }
		return okIf(true)
	case *schema.ObjectType:
		// We implement structural typing for objects.
		from, ok := from.(*schema.ObjectType)
		if !ok {
			return fail
		}
		failures := []*notAssignable{}
		for _, prop := range to.Properties {
			fromProp, ok := from.Property(prop.Name)
			if prop.IsRequired() && !ok {
				failures = append(failures, (&notAssignable{
					reason: fmt.Sprintf("Missing required property '%s'", prop.Name),
				}).Property(prop.Name))
				continue
			}
			if !ok {
				// We don't have an optional property, which is ok
				continue
			}
			// We have a matching property, so the type must agree
			notAssignable := isAssignable(fromProp.Type, prop.Type)
			if notAssignable != nil {
				failures = append(failures, notAssignable.Property(prop.Name))
				continue
			}
		}
		return okIf(len(failures) == 0).Because(failures...)

	case *schema.TokenType:
		if to.UnderlyingType != nil {
			return okIfAssignable(isAssignable(from, to.UnderlyingType))
		}
		return okIfAssignable(isAssignable(from, schema.AnyType))

	default:
		// We mark this an internal error since we don't recognize the type.
		// This means we are missing a type, not that the user has an invalid
		// program.
		return &notAssignable{reason: fmt.Sprintf("Unknown type: %[1]s (%[1]T)", to), internal: true}
	}
}

// Provides an appropriate diagnostic message if it is illegal to assign `from`
// to `to`.
func assertTypeAssignable(ctx *evalContext, loc *hcl.Range, from, to schema.Type) {
	if to == nil {
		return
	}
	result := isAssignable(from, to)
	if result == nil {
		return
	}
	summary := fmt.Sprintf("%s is not assignable from %s", displayType(to), displayType(from))
	if result.IsInternal() {
		ctx.addWarnDiag(loc, fmt.Sprintf("internal error: %s", summary), result.String())
		return
	}
	ctx.addErrDiag(loc, summary, result.String())
}

func (tc *typeCache) typeResource(r *Runner, node resourceNode) bool {
	k, v := node.Key.Value, node.Value
	ctx := r.newContext(node)
	pkg, typ, err := ResolveResource(ctx.pkgLoader, v.Type.Value)
	if err != nil {
		ctx.sdiags.diags.Extend(syntax.NodeError(v.Syntax(), fmt.Sprintf("error resolving type of resource %v: %v", k, err), ""))
		ctx.error(v.Type, fmt.Sprintf("error resolving type of resource %v: %v", k, err))
		return true
	}
	hint := pkg.ResourceTypeHint(typ)
	var allProperties []string
	for _, prop := range hint.Resource.InputProperties {
		allProperties = append(allProperties, prop.Name)
	}
	fmtr := yamldiags.NonExistantFieldFormatter{
		ParentLabel:         fmt.Sprintf("Resource %s", typ.String()),
		Fields:              allProperties,
		MaxElements:         5,
		FieldsAreProperties: true,
	}

	tc.typePropertyEntries(ctx, k, fmtr, v.Properties.Entries, hint.Resource.InputProperties)

	tc.registerResource(k, node.Value, hint)

	if len(v.Properties.Entries) > 0 && (v.Get.Id != nil || len(v.Get.State.Entries) > 0) {
		ctx.addErrDiag(node.Key.Syntax().Syntax().Range(),
			"Resource fields properties and get are mutually exclusive",
			"Properties is used to describe a resource managed by Pulumi.\n"+
				"Get is used to describe a resource managed outside of the current Pulumi stack.\n"+
				"See https://www.pulumi.com/docs/intro/concepts/resources/get for more details on using Get.",
		)
	}

	if existing, ok := tc.exprs[v.Get.Id]; ok && v.Get.Id != nil {
		assertTypeAssignable(ctx, v.Get.Id.Syntax().Syntax().Range(), existing, schema.StringType)
	}

	statePropNames := []string{}
	for _, prop := range hint.Resource.Properties {
		statePropNames = append(statePropNames, prop.Name)
	}
	fmtr = yamldiags.NonExistantFieldFormatter{
		ParentLabel:         fmt.Sprintf("Resource %s", typ.String()),
		Fields:              statePropNames,
		MaxElements:         5,
		FieldsAreProperties: true,
	}
	tc.typePropertyEntries(ctx, k, fmtr, v.Get.State.Entries, hint.Resource.Properties)

	// Check for extra fields that didn't make it into the resource or resource options object
	options := ResourceOptionsTypeHint()
	allOptions := make([]string, 0, len(options))
	for k := range options {
		allOptions = append(allOptions, k)
	}
	if s := v.Syntax(); s != nil {
		if o, ok := s.(*syntax.ObjectNode); ok {
			validKeys := append(v.Fields(), "condition", "metadata")
			fmtr := yamldiags.InvalidFieldBagFormatter{
				ParentLabel: fmt.Sprintf("Resource %s", typ.String()),
				MaxListed:   5,
				Bags: []yamldiags.TypeBag{
					{Name: "properties", Properties: allProperties},
					{Name: "options", Properties: allOptions},
					{Name: "get", Properties: []string{"id", "state"}},
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
				ctx.addErrDiag(subject, summary, detail)
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
				ctx.addErrDiag(subject, summary, detail)
			}
		}
	}

	return true
}

func (tc *typeCache) typePropertyEntries(ctx *evalContext, resourceName string, fmtr yamldiags.NonExistantFieldFormatter, entries []ast.PropertyMapEntry, props []*schema.Property) {
	propMap := map[string]*schema.Property{}
	for _, p := range props {
		propMap[p.Name] = p
	}
	for _, kvp := range entries {
		if typ, hasField := propMap[kvp.Key.Value]; !hasField {
			summary, detail := fmtr.MessageWithDetail(kvp.Key.Value, fmt.Sprintf("Property %s", kvp.Key.Value))
			subject := kvp.Key.Syntax().Syntax().Range()
			ctx.addErrDiag(subject, summary, detail)
		} else {
			existing, ok := tc.exprs[kvp.Value]
			rng := kvp.Key.Syntax().Syntax().Range()
			if !ok {
				ctx.addWarnDiag(rng,
					fmt.Sprintf("internal error: untyped input for %s.%s", resourceName, kvp.Key.Value),
					fmt.Sprintf("expected type %s", typ.Type))
			} else if typ.Type == nil {
				ctx.addWarnDiag(rng,
					fmt.Sprintf("internal error: unable to discover expected type for %s.%s", resourceName, kvp.Key.Value),
					fmt.Sprintf("got type %s", existing))
			} else {
				assertTypeAssignable(ctx, rng, existing, typ.Type)
			}
		}
	}
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
				ctx.addWarnDiag(subject, summary, detail)
			} else {
				tc.exprs[prop.Value] = typ
			}
		}
	}
	if t.CallOpts.Parent != nil {
		tc.typeExpr(ctx, t.CallOpts.Parent)
	}
	if t.CallOpts.Provider != nil {
		tc.typeExpr(ctx, t.CallOpts.Provider)
	}
	if t.CallOpts.Version != nil {
		tc.typeExpr(ctx, t.CallOpts.Version)
	}
	if t.CallOpts.PluginDownloadURL != nil {
		tc.typeExpr(ctx, t.CallOpts.PluginDownloadURL)
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
			ctx.addErrDiag(t.Return.Syntax().Syntax().Range(), summary, detail)
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
	if root, ok := tc.resourceNames[t.Property.RootName()]; ok {
		typ = tc.resources[root]
	}
	if root, ok := tc.variableNames[t.Property.RootName()]; ok {
		typ = tc.exprs[root]
	}
	if root, ok := tc.configuration[t.Property.RootName()]; ok {
		typ = root
	}
	runningName := t.Property.RootName()
	setError := func(summary, detail string) *schema.InvalidType {
		diag := syntax.Error(t.Syntax().Syntax().Range(), summary, detail)
		ctx.addErrDiag(t.Syntax().Syntax().Range(), summary, detail)
		typ := &schema.InvalidType{
			Diagnostics: []*hcl.Diagnostic{diag.HCL()},
		}
		return typ
	}

	tc.exprs[t] = typePropertyAccess(ctx, typ, runningName, t.Property.Accessors[1:], setError)
	return true
}

func typePropertyAccess(ctx *evalContext, root schema.Type,
	runningName string, accessors []ast.PropertyAccessor,
	setError func(summary, detail string) *schema.InvalidType) schema.Type {
	if len(accessors) == 0 {
		return root
	}
	if root, ok := root.(*schema.UnionType); ok {
		var possibilities OrderedTypeSet
		errs := []*notAssignable{}
		for _, subtypes := range root.ElementTypes {
			t := typePropertyAccess(ctx, subtypes, runningName, accessors,
				func(summary, detail string) *schema.InvalidType {
					errs = append(errs, &notAssignable{reason: summary, property: subtypes.String()})
					return &schema.InvalidType{}
				})
			if _, ok := t.(*schema.InvalidType); !ok {
				possibilities.Add(t)
			}
		}
		if len(errs) > 0 {
			op := "access"
			if _, ok := accessors[0].(*ast.PropertySubscript); ok {
				op = "index"
			}
			setError(fmt.Sprintf("Cannot %s into %s of type %s", op, runningName, displayType(root)),
				notAssignable{
					reason:  fmt.Sprintf("'%s' could be a type that does not support %sing", runningName, op),
					because: errs,
				}.String())
			return &schema.InvalidType{}
		}
		if possibilities.Len() == 1 {
			return possibilities.First()
		}

		return &schema.UnionType{ElementTypes: possibilities.Values()}
	}
	switch accessor := accessors[0].(type) {
	case *ast.PropertyName:
		properties := map[string]schema.Type{}
		switch root := codegen.UnwrapType(root).(type) {
		case *schema.ObjectType:
			for _, prop := range root.Properties {
				properties[prop.Name] = prop.Type
			}
		case *schema.ResourceType:
			for _, prop := range root.Resource.Properties {
				properties[prop.Name] = prop.Type
			}
			if !root.Resource.IsComponent {
				properties["id"] = schema.StringType
			}
			properties["urn"] = schema.StringType
		case *schema.InvalidType:
			return root
		default:
			return setError(
				fmt.Sprintf("cannot access a property on '%s' (type %s)", runningName,
					displayType(codegen.UnwrapType(root))),
				"Property access is only allowed on Resources and Objects",
			)
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
			return setError(summary, detail)
		}
		return typePropertyAccess(ctx, newType, runningName+"."+accessor.Name, accessors[1:], setError)
	case *ast.PropertySubscript:
		err := func(typ, msg string) *schema.InvalidType {
			return setError(
				fmt.Sprintf("Cannot index%s into '%s' (type %s)", typ, runningName,
					displayType(codegen.UnwrapType(root))),
				msg,
			)
		}
		switch root := codegen.UnwrapType(root).(type) {
		case *schema.ArrayType:
			if _, ok := accessor.Index.(string); ok {
				return err(" via string", "Index via string is only allowed on Maps")
			}
			return typePropertyAccess(ctx, root.ElementType,
				runningName+fmt.Sprintf("[%d]", accessor.Index.(int)),
				accessors[1:], setError)
		case *schema.MapType:
			if _, ok := accessor.Index.(int); ok {
				return err(" via number", "Index via number is only allowed on Arrays")
			}
			return typePropertyAccess(ctx, root.ElementType,
				runningName+fmt.Sprintf("[%q]", accessor.Index.(string)),
				accessors[1:], setError)
		case *schema.InvalidType:
			return &schema.InvalidType{}
		default:
			return err("", "Index property access is only allowed on Maps and Lists")
		}
	default:
		panic(fmt.Sprintf("Unknown property type: %T", accessor))
	}
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
		var types OrderedTypeSet
		for _, typ := range t.Elements {
			types.Add(tc.exprs[typ])
		}

		var elementType schema.Type
		switch types.Len() {
		case 0:
			elementType = &schema.InvalidType{}
		case 1:
			elementType = types.First()
		default:
			elementType = &schema.UnionType{
				ElementTypes: types.Values(),
			}
		}

		tc.exprs[t] = &schema.ArrayType{
			ElementType: elementType,
		}
	case *ast.ObjectExpr:
		// This is an add hock object
		properties := make([]*schema.Property, 0, len(t.Entries))
		propNames := make([]string, 0, len(t.Entries))
		for _, entry := range t.Entries {
			k, v := entry.Key.(*ast.StringExpr), entry.Value
			properties = append(properties, &schema.Property{
				Name: k.Value,
				Type: tc.exprs[v],
			})
			propNames = append(propNames, k.Value)
		}
		tc.exprs[t] = &schema.ObjectType{
			Token:      adhockObjectToken + strings.Join(propNames, "•"),
			Properties: properties,
		}
	case *ast.NullExpr:
		tc.exprs[t] = &schema.InvalidType{}
	case *ast.SecretExpr:
		// The type of a secret is the type of its argument
		tc.exprs[t] = tc.exprs[t.Value]
	case *ast.SplitExpr:
		assertTypeAssignable(ctx, t.Delimiter.Syntax().Syntax().Range(), tc.exprs[t.Delimiter], schema.StringType)
		assertTypeAssignable(ctx, t.Source.Syntax().Syntax().Range(), tc.exprs[t.Source], schema.StringType)
		tc.exprs[t] = &schema.ArrayType{ElementType: schema.StringType}
	case *ast.SelectExpr:
		assertTypeAssignable(ctx, t.Index.Syntax().Syntax().Range(), tc.exprs[t.Index], schema.IntType)
		assertTypeAssignable(ctx, t.Values.Syntax().Syntax().Range(), tc.exprs[t.Values],
			&schema.ArrayType{ElementType: schema.AnyType}) // We accept an array of any type
		if valuesType, ok := tc.exprs[t.Values]; ok {
			arr, ok := codegen.UnwrapType(valuesType).(*schema.ArrayType)
			if ok {
				tc.exprs[t] = arr.ElementType
			} else {
				tc.exprs[t] = &schema.InvalidType{
					Diagnostics: []*hcl.Diagnostic{
						{Summary: fmt.Sprintf("Could not derive types from array, since a %T was found instead", arr)},
					}}

			}
		} else {
			tc.exprs[t] = &schema.InvalidType{
				Diagnostics: []*hcl.Diagnostic{
					{Summary: fmt.Sprintf("Could not derive types from array, since a %T was found instead", t.Values)},
				}}
		}
	default:
		tc.exprs[t] = &schema.InvalidType{
			Diagnostics: []*hcl.Diagnostic{{Summary: fmt.Sprintf("Hit unknown type: %T", t)}},
		}
	}
	return true
}

func (tc *typeCache) typeVariable(r *Runner, node variableNode) bool {
	k, v := node.Key.Value, node.Value
	tc.variableNames[k] = v
	return true
}

func (tc *typeCache) typeConfig(r *Runner, node configNode) bool {
	k, v := node.Key.Value, node.Value
	var typ schema.Type = &schema.InvalidType{}
	var optional bool
	switch {
	case v.Default != nil:
		// We have a default, so the type is optional
		typ = tc.exprs[v.Default]
		optional = true
	case v.Type != nil:
		ctype, ok := ctypes.Parse(v.Type.Value)
		if ok {
			typ = ctype.Schema()
		}
	}
	typ = &schema.InputType{ElementType: typ}
	if optional {
		typ = &schema.OptionalType{ElementType: typ}
	}
	tc.configuration[k] = typ
	return true
}

func (tc *typeCache) typeOutput(r *Runner, node ast.PropertyMapEntry) bool {
	tc.outputs[node.Key.Value] = tc.exprs[node.Value]
	return true
}

func newTypeCache() *typeCache {
	pulumiExpr := ast.Object(
		ast.ObjectProperty{Key: ast.String("cwd")},
		ast.ObjectProperty{Key: ast.String("project")},
		ast.ObjectProperty{Key: ast.String("stack")},
	)
	return &typeCache{
		exprs: map[ast.Expr]schema.Type{
			pulumiExpr: &schema.ObjectType{
				Token: "pulumi:builtin:pulumi",
				Properties: []*schema.Property{
					{Name: "cwd", Type: schema.StringType},
					{Name: "project", Type: schema.StringType},
					{Name: "stack", Type: schema.StringType},
				},
			},
		},
		resources:     map[*ast.ResourceDecl]schema.Type{},
		configuration: map[string]schema.Type{},
		resourceNames: map[string]*ast.ResourceDecl{},
		variableNames: map[string]ast.Expr{
			PulumiVarName: pulumiExpr,
		},
		outputs: map[string]schema.Type{},
	}
}

func TypeCheck(r *Runner) (Typing, syntax.Diagnostics) {
	types := newTypeCache()

	// Set roots
	diags := r.Run(walker{
		VisitResource: types.typeResource,
		VisitExpr:     types.typeExpr,
		VisitVariable: types.typeVariable,
		VisitConfig:   types.typeConfig,
		VisitOutput:   types.typeOutput,
	})

	return types, diags
}

type walker struct {
	VisitConfig   func(r *Runner, node configNode) bool
	VisitVariable func(r *Runner, node variableNode) bool
	VisitOutput   func(r *Runner, node ast.PropertyMapEntry) bool
	VisitResource func(r *Runner, node resourceNode) bool
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

func (e walker) EvalConfig(r *Runner, node configNode) bool {
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
	if e.VisitConfig != nil {
		if !e.VisitConfig(r, node) {
			return false
		}
	}
	return true
}
func (e walker) EvalVariable(r *Runner, node variableNode) bool {
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
func (e walker) EvalOutput(r *Runner, node ast.PropertyMapEntry) bool {
	if e.VisitExpr != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		if !e.walk(ctx, node.Value) {
			return false
		}
	}

	if e.VisitOutput != nil {
		if !e.VisitOutput(r, node) {
			return false
		}
	}
	return true
}
func (e walker) EvalResource(r *Runner, node resourceNode) bool {
	if e.VisitExpr != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		v := node.Value
		if !e.walk(ctx, v.Type) {
			return false
		}
		if !e.walkPropertyMap(ctx, v.Properties) {
			return false
		}
		if !e.walkResourceOptions(ctx, v.Options) {
			return false
		}
		if !e.walkGetResoure(ctx, v.Get) {
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

func (e walker) walkPropertyMap(ctx *evalContext, m ast.PropertyMapDecl) bool {
	for _, prop := range m.Entries {
		if !e.walk(ctx, prop.Key) {
			return false
		}
		if !e.walk(ctx, prop.Value) {
			return false
		}
	}
	return true
}

func (e walker) walkGetResoure(ctx *evalContext, get ast.GetResourceDecl) bool {
	if !e.walk(ctx, get.Id) {
		return false
	}
	return e.walkPropertyMap(ctx, get.State)
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

type OrderedTypeSet struct {
	// Provide O(1) existence checking
	existence map[schema.Type]struct{}
	// Provide ordering
	order []schema.Type
}

func (o *OrderedTypeSet) Add(t schema.Type) bool {
	if o.existence == nil {
		o.existence = map[schema.Type]struct{}{}
	}
	_, ok := o.existence[t]
	if ok {
		return false
	}
	o.existence[t] = struct{}{}
	o.order = append(o.order, t)
	return true
}

func (o *OrderedTypeSet) Values() []schema.Type {
	a := make([]schema.Type, 0, len(o.order))
	return append(a, o.order...)
}

func (o *OrderedTypeSet) Len() int {
	// This will equal len(o.existence) assuming that the internals have not been messed with.
	return len(o.order)
}

// First returns the first element added, panicking if no element exists.
func (o *OrderedTypeSet) First() schema.Type {
	return o.order[0]
}
