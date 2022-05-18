// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

type typeCache struct {
	exprs     map[ast.Expr]TypeHint
	resources map[*ast.ResourceDecl]TypeHint
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
	fields := hint.InputProperties()
	var allProperties []string
	for k := range fields {
		allProperties = append(allProperties, k)
	}
	fmtr := yamldiags.NonExistantFieldFormatter{
		ParentLabel:         fmt.Sprintf("Resource %s", typ.String()),
		Fields:              allProperties,
		MaxElements:         5,
		FieldsAreProperties: true,
	}

	for _, kvp := range v.Properties.Entries {
		if typ, hasField := fields[kvp.Key.Value]; !hasField {
			summary, detail := fmtr.MessageWithDetail(kvp.Key.Value, fmt.Sprintf("Property %s", kvp.Key.Value))
			subject := kvp.Key.Syntax().Syntax().Range()
			valueRange := kvp.Value.Syntax().Syntax().Range()
			context := hcl.RangeOver(*subject, *valueRange)
			ctx.addDiag(syntax.Error(subject, summary, detail).WithContext(&context))
		} else {
			tc.exprs[kvp.Value] = typ
		}
	}
	tc.resources[node.Value] = hint.Fields()
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
	inputs := hint.InputProperties()
	for k := range inputs {
		existing = append(existing, k)
	}
	fmtr := yamldiags.NonExistantFieldFormatter{
		ParentLabel: fmt.Sprintf("Invoke %s", functionName.String()),
		Fields:      existing,
		MaxElements: 5,
	}
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
	tc.exprs[t] = hint.Fields()
	return true
}

func (tc *typeCache) typeExpr(ctx *evalContext, t ast.Expr) bool {
	switch t := t.(type) {
	case *ast.InvokeExpr:
		return tc.typeInvoke(ctx, t)
	default:
		return true
	}
}

func newTypeCache() *typeCache {
	return &typeCache{
		exprs:     map[ast.Expr]TypeHint{},
		resources: map[*ast.ResourceDecl]TypeHint{},
	}
}

func TypeCheck(r *runner) syntax.Diagnostics {
	types := newTypeCache()

	// Set roots
	diags := r.Run(walker{
		VisitResource: types.typeResource,
		VisitExpr:     types.typeExpr,
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
	if !e.VisitExpr(ctx, x) {
		return false
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
	if e.VisitVariable != nil {
		if !e.VisitVariable(r, node) {
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
	if e.VisitResource != nil {
		if !e.VisitResource(r, node) {
			return false
		}
	}
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
