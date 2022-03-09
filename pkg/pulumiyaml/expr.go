// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

// GetResourceDependencies gets the full set of implicit and explicit dependencies for a Resource.
func GetResourceDependencies(r *ast.ResourceDecl) []*ast.StringExpr {
	var deps []*ast.StringExpr
	for _, kvp := range r.Properties.Entries {
		getExpressionDependencies(&deps, kvp.Value)
	}
	if r.Options.DependsOn != nil {
		getExpressionDependencies(&deps, r.Options.DependsOn)
	}
	if r.Options.Parent != nil {
		getExpressionDependencies(&deps, r.Options.Parent)
	}
	if r.Options.Provider != nil {
		getExpressionDependencies(&deps, r.Options.Provider)
	}
	if r.Options.Providers != nil {
		getExpressionDependencies(&deps, r.Options.Providers)
	}
	return deps
}

// GetVariableDependencies gets the full set of implicit and explicit dependencies for a Variable.
func GetVariableDependencies(e ast.VariablesMapEntry) []*ast.StringExpr {
	var deps []*ast.StringExpr
	getExpressionDependencies(&deps, e.Value)
	return deps
}

// getResourceDependencies gets the resource dependencies of an expression.
func getExpressionDependencies(deps *[]*ast.StringExpr, x ast.Expr) {
	switch x := x.(type) {
	case *ast.ListExpr:
		for _, e := range x.Elements {
			getExpressionDependencies(deps, e)
		}
	case *ast.ObjectExpr:
		for _, kvp := range x.Entries {
			getExpressionDependencies(deps, kvp.Key)
			getExpressionDependencies(deps, kvp.Value)
		}
	case *ast.InterpolateExpr:
		for _, p := range x.Parts {
			if p.Value != nil {
				name := p.Value.Accessors[0].(*ast.PropertyName).Name
				sx := ast.StringSyntax(syntax.StringSyntax(x.Syntax().Syntax(), name))
				*deps = append(*deps, sx)
			}
		}
	case *ast.SymbolExpr:
		name := x.Property.Accessors[0].(*ast.PropertyName).Name
		sx := ast.StringSyntax(syntax.StringSyntax(x.Syntax().Syntax(), name))
		*deps = append(*deps, sx)
	case *ast.RefExpr:
		*deps = append(*deps, x.ResourceName)
	case *ast.GetAttExpr:
		*deps = append(*deps, x.ResourceName)
	case ast.BuiltinExpr:
		getExpressionDependencies(deps, x.Args())

	}
}
