// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

func topologicallySortedResources(t *ast.TemplateDecl) ([]ast.ResourcesMapEntry, syntax.Diagnostics) {
	if t.Resources == nil {
		return nil, nil
	}

	var diags syntax.Diagnostics

	var sorted []ast.ResourcesMapEntry // will hold the sorted vertices.
	visiting := map[string]bool{}      // temporary entries to detect cycles.
	visited := map[string]bool{}       // entries to avoid visiting the same node twice.

	// Precompute dependencies for each resource
	intermediates := map[string]ast.Intermediate{}
	dependencies := map[string][]*ast.StringExpr{}
	for _, kvp := range t.Resources.Entries {
		rname, r := kvp.Key.Value, kvp.Value
		if _, has := intermediates[rname]; has {
			diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("duplicate resource name '%s'", rname), ""))
			continue
		}
		temp := kvp
		//nolint:govet // safety: we control this composite type
		intermediates[rname] = ast.Intermediate{&temp}
		dependencies[rname] = GetResourceDependencies(r)
	}
	if t.Configuration != nil {
		for _, kvp := range t.Configuration.Entries {
			cname := kvp.Key.Value
			// Prevent aliasing, see: http://blogs.msdn.com/b/ericlippert/archive/tags/closures/
			temp := kvp
			//nolint:govet // safety: we control this composite type
			intermediates[cname] = ast.Intermediate{&temp}
			dependencies[cname] = make([]*ast.StringExpr, 0)
		}
	}
	if t.Variables != nil {
		for _, kvp := range t.Variables.Entries {
			vname := kvp.Key.Value
			if _, has := intermediates[vname]; has {
				diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("duplicate resource name '%s'", vname), ""))
				continue
			}
			if _, found := intermediates[vname]; found {
				diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("variable %s cannot have the same name as a resource", vname), ""))
				return nil, diags
			}
			// Prevent aliasing, see: http://blogs.msdn.com/b/ericlippert/archive/tags/closures/
			temp := kvp
			//nolint:govet // safety: we control this composite type
			intermediates[vname] = ast.Intermediate{&temp}
			dependencies[vname] = GetVariableDependencies(&temp)
		}
	}

	// Depth-first visit each node
	var visit func(name *ast.StringExpr) bool
	visit = func(name *ast.StringExpr) bool {
		e, ok := intermediates[name.Value]
		if !ok {
			diags.Extend(ast.ExprError(
				name,
				fmt.Sprintf("dependency %s not found", name.Value),
				"",
			))
			return false
		}
		kind := e.Kind()

		if visiting[name.Value] {
			diags.Extend(ast.ExprError(
				name,
				fmt.Sprintf("circular dependency of %s '%s' transitively on itself", kind, name.Value),
				"",
			))
			return false
		}
		if !visited[name.Value] {
			visiting[name.Value] = true
			for _, mname := range dependencies[name.Value] {
				if !visit(mname) {
					return false
				}
			}
			visited[name.Value] = true
			visiting[name.Value] = false

			e.Visit(ast.IntermediateVisitor{
				VisitResource: func(r *ast.ResourcesMapEntry) {
					sorted = append(sorted, *r)
				},
				VisitVariable: func(_ *ast.VariablesMapEntry) {
					// Variables participate in the topological sort only as intermediate values, but are
					// elided from the output.
				},
				VisitConfig: func(_ *ast.ConfigMapEntry) {},
			})
		}
		return true
	}

	// Repeatedly visit the first unvisited unode until none are left
	for {
		progress := false
		for _, e := range intermediates {
			if !visited[e.Key().Value] {
				if visit(e.Key()) {
					progress = true
				}
				break
			}
		}
		if !progress {
			break
		}
	}
	return sorted, diags
}
