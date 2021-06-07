package pulumiyaml

import (
	"github.com/pulumi/pulumi-yaml/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pulumiyaml/syntax"
)

// GetResourceDependencies gets the full set of implicit and explicit dependencies for a Resource.
func GetResourceDependencies(r *ast.ResourceDecl) []*ast.StringExpr {
	var deps []*ast.StringExpr
	for _, kvp := range r.Properties.Entries {
		getExpressionDependencies(&deps, kvp.Value)
	}
	if r.DependsOn != nil {
		for _, r := range r.DependsOn.Elements {
			deps = append(deps, r)
		}
	}
	if r.Provider != nil && r.Provider.Value != "" {
		deps = append(deps, r.Provider)
	}
	if r.Parent != nil && r.Parent.Value != "" {
		deps = append(deps, r.Parent)
	}
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
				getExpressionDependencies(deps, &ast.SymbolExpr{Property: p.Value})
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
