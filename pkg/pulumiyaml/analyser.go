// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

type typeCache map[ast.Expr]TypeHint

func (tc *typeCache) insert(e ast.Expr, t TypeHint) {
	map[ast.Expr]TypeHint(*tc)[e] = t
}

func (tc *typeCache) get(e ast.Expr) (TypeHint, bool) {
	t, ok := map[ast.Expr]TypeHint(*tc)[e]
	return t, ok
}

func (tc *typeCache) anchorResource(r *runner, node resourceNode) bool {
	k, v := node.Key.Value, node.Value
	ctx := r.newContext(node)
	pkg, typ, err := ResolveResource(ctx.pkgLoader, v.Type.Value)
	if err != nil {
		ctx.error(v.Type, fmt.Sprintf("error resolving type of resource %v: %v", k, err))
		return true
	}
	fields := pkg.ResourceTypeHint(typ).InputProperties()
	var allProperties []string
	for k := range fields {
		allProperties = append(allProperties, k)
	}
	fmtr := yamldiags.NonExistantFieldFormatter{
		ParentLabel:         fmt.Sprintf("Resource '%s'", typ.String()),
		Fields:              allProperties,
		MaxElements:         5,
		FieldsAreProperties: true,
	}

	for _, kvp := range v.Properties.Entries {
		if typ, hasField := fields[kvp.Key.Value]; !hasField {
			summary, detail := fmtr.MessageWithDetail(kvp.Key.Value, fmt.Sprintf("Property '%s'", kvp.Key.Value))
			subject := kvp.Key.Syntax().Syntax().Range()
			valueRange := kvp.Value.Syntax().Syntax().Range()
			context := hcl.RangeOver(*subject, *valueRange)
			ctx.addDiag(&syntax.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     summary,
				Detail:      detail,
				Subject:     subject,
				Context:     &context,
				Expression:  nil,
				EvalContext: &hcl.EvalContext{},
			})
		} else {
			tc.insert(kvp.Value, typ)
		}
	}
	return true
}

func (tc *typeCache) anchorInvoke(ctx *evalContext, t *ast.InvokeExpr) bool {
	pkg, functionName, err := ResolveFunction(ctx.pkgLoader, t.Token.Value)
	if err != nil {
		_, b := ctx.error(t, err.Error())
		return b
	}
	var existing []string
	inputs := pkg.FunctionTypeHint(functionName).InputProperties()
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
			msg, detail := fmtr.MessageWithDetail(k, k)
			ctx.addDiag(&syntax.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  msg,
				Detail:   detail,
				Subject:  prop.Key.Syntax().Syntax().Range(),
				Context:  t.Syntax().Syntax().Range(),
			})
		} else {
			tc.insert(prop.Value, typ)
		}
	}
	return true
}

func (tc *typeCache) anchorExpr(ctx *evalContext, t ast.Expr) bool {
	switch t := t.(type) {
	case *ast.InvokeExpr:
		return tc.anchorInvoke(ctx, t)
	default:
		return true
	}
}

func TypeCheck(r *runner) syntax.Diagnostics {
	types := typeCache{}

	// Set roots
	diags := r.Run(walker{
		EvalResourceFunc: types.anchorResource,
		EvalExprFunc:     types.anchorExpr,
	})

	return diags
}

type walker struct {
	EvalConfigFunc   func(r *runner, node configNode) bool
	EvalVariableFunc func(r *runner, node variableNode) bool
	EvalOutputFunc   func(r *runner, node ast.PropertyMapEntry) bool
	EvalResourceFunc func(r *runner, node resourceNode) bool
	EvalExprFunc     func(*evalContext, ast.Expr) bool
}

func (e walker) walk(ctx *evalContext, x ast.Expr) bool {
	if x == nil {
		return true
	}
	if !e.EvalExprFunc(ctx, x) {
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
	case *ast.InvokeExpr:
		if !e.walk(ctx, x.Token) {
			return false
		}
		if !e.walk(ctx, x.CallArgs) {
			return false
		}
		return e.walk(ctx, x.Return)
	case *ast.JoinExpr:
		if !e.walk(ctx, x.Delimiter) {
			return false
		}
		return e.walk(ctx, x.Values)
	case *ast.SplitExpr:
		if !e.walk(ctx, x.Delimiter) {
			return false
		}
		return e.walk(ctx, x.Source)
	case *ast.ToJSONExpr:
		return e.walk(ctx, x.Value)
	case *ast.SelectExpr:
		if !e.walk(ctx, x.Index) {
			return false
		}
		return e.walk(ctx, x.Values)
	case *ast.ToBase64Expr:
		return e.walk(ctx, x.Value)
	case *ast.FileAssetExpr:
		return e.walk(ctx, x.Source)
	case *ast.StringAssetExpr:
		return e.walk(ctx, x.Source)
	case *ast.RemoteAssetExpr:
		return e.walk(ctx, x.Source)
	case *ast.FileArchiveExpr:
		return e.walk(ctx, x.Source)
	case *ast.RemoteArchiveExpr:
		return e.walk(ctx, x.Source)
	case *ast.AssetArchiveExpr:
		for _, v := range x.AssetOrArchives {
			if !e.walk(ctx, v) {
				return false
			}
		}
	case *ast.StackReferenceExpr:
		if !e.walk(ctx, x.PropertyName) {
			return false
		}
		return e.walk(ctx, x.StackName)
	default:
		panic(fmt.Sprintf("fatal: invalid expr type %T", x))
	}
	return true
}

func (e walker) EvalConfig(r *runner, node configNode) bool {
	if e.EvalConfigFunc != nil {
		if !e.EvalConfigFunc(r, node) {
			return false
		}
	}
	if e.EvalExprFunc != nil {
		ctx := r.newContext(node)
		if !e.walk(ctx, node.Key) {
			return false
		}
		c := node.Value
		if !e.walk(ctx, c.Secret) {
			return false
		}
		if !e.walk(ctx, c.Default) {
			return false
		}
	}
	return true
}
func (e walker) EvalVariable(r *runner, node variableNode) bool {
	if e.EvalVariableFunc != nil {
		if !e.EvalVariableFunc(r, node) {
			return false
		}
	}
	if e.EvalExprFunc != nil {
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
	if e.EvalOutputFunc != nil {
		if !e.EvalOutputFunc(r, node) {
			return false
		}
	}
	if e.EvalExprFunc != nil {
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
	if e.EvalResourceFunc != nil {
		if !e.EvalResourceFunc(r, node) {
			return false
		}
	}
	if e.EvalExprFunc != nil {
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
