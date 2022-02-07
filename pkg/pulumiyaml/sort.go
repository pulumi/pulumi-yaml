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
	resources := map[string]ast.ResourcesMapEntry{}
	dependencies := map[string][]*ast.StringExpr{}
	for _, kvp := range t.Resources.Entries {
		rname, r := kvp.Key.Value, kvp.Value
		if _, has := resources[rname]; has {
			diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("duplicate resource name '%s'", rname), ""))
			continue
		}
		resources[rname] = kvp
		dependencies[rname] = GetResourceDependencies(r)
	}

	// Depth-first visit each node
	var visit func(name *ast.StringExpr) bool
	visit = func(name *ast.StringExpr) bool {
		if visiting[name.Value] {
			diags.Extend(ast.ExprError(
				name,
				fmt.Sprintf("circular dependency of resource '%s' transitively on itself", name.Value),
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

			if r, ok := resources[name.Value]; ok {
				sorted = append(sorted, r)
			}
		}
		return true
	}

	// Repeatedly visit the first unvisited unode until none are left
	for {
		progress := false
		for _, kvp := range t.Resources.Entries {
			if !visited[kvp.Key.Value] {
				if visit(kvp.Key) {
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
