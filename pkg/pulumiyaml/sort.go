// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

//see: https://github.com/BurntSushi/go-sumtype
//go-sumtype:decl IntermediateSymbol

type graphNode interface {
	valueKind() string
	key() *ast.StringExpr
}

type resourceNode ast.ResourcesMapEntry

func (e resourceNode) valueKind() string {
	return "resource"
}

func (e resourceNode) key() *ast.StringExpr {
	return e.Key
}

type variableNode ast.VariablesMapEntry

func (e variableNode) valueKind() string {
	return "variable"
}

func (e variableNode) key() *ast.StringExpr {
	return e.Key
}

type configNode ast.ConfigMapEntry

func (e configNode) valueKind() string {
	return "config"
}

func (e configNode) key() *ast.StringExpr {
	return e.Key
}

func topologicallySortedResources(t *ast.TemplateDecl) ([]graphNode, map[string][]graphNode, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	var sorted []graphNode        // will hold the sorted vertices.
	visiting := map[string]bool{} // temporary entries to detect cycles.
	visited := map[string]bool{}  // entries to avoid visiting the same node twice.

	// Precompute dependencies for each resource
	intermediates := map[string]graphNode{}
	dependencies := map[string][]*ast.StringExpr{}
	for _, kvp := range t.Configuration.Entries {
		cname := kvp.Key.Value
		node := configNode(kvp)

		cdiags := checkUniqueNode(intermediates, node)
		diags = append(diags, cdiags...)

		if !cdiags.HasErrors() {
			intermediates[cname] = node
			dependencies[cname] = nil

			// Special case: configuration goes first
			visited[cname] = true
			sorted = append(sorted, node)
		}
	}
	for _, kvp := range t.Resources.Entries {
		rname, r := kvp.Key.Value, kvp.Value
		node := resourceNode(kvp)

		cdiags := checkUniqueNode(intermediates, node)
		diags = append(diags, cdiags...)

		if !cdiags.HasErrors() {
			intermediates[rname] = node
			dependencies[rname] = GetResourceDependencies(r)
		}
	}
	for _, kvp := range t.Variables.Entries {
		vname := kvp.Key.Value
		node := variableNode(kvp)

		cdiags := checkUniqueNode(intermediates, node)
		diags = append(diags, cdiags...)

		if !cdiags.HasErrors() {
			intermediates[vname] = node
			dependencies[vname] = GetVariableDependencies(kvp)
		}
	}

	if diags.HasErrors() {
		return nil, nil, diags
	}

	// Compute transitive dependencies for resources:
	exists := struct{}{}
	transitiveDependencies := map[string]map[string]struct{}{}

	// Depth-first visit each node
	var visit func(name *ast.StringExpr) bool
	visit = func(name *ast.StringExpr) bool {
		// Special case: pulumi variable has no dependencies.
		if name.Value == PulumiVarName {
			visited[PulumiVarName] = true
			return true
		}

		e, ok := intermediates[name.Value]
		if !ok {
			diags.Extend(ast.ExprError(name, fmt.Sprintf("resource %s not found", name.Value), ""))
			return false
		}
		kind := e.valueKind()

		if visiting[name.Value] {
			diags.Extend(ast.ExprError(
				name,
				fmt.Sprintf("circular dependency of %s '%s' transitively on itself", kind, name.Value),
				"",
			))
			return false
		}
		if !visited[name.Value] {
			transitiveDependencies[name.Value] = map[string]struct{}{}
			visiting[name.Value] = true
			for _, mname := range dependencies[name.Value] {
				if !visit(mname) {
					return false
				}
				if intermediates[mname.Value].valueKind() == "resource" {
					transitiveDependencies[name.Value][mname.Value] = exists
				} else {
					for tDep := range transitiveDependencies[mname.Value] {
						transitiveDependencies[name.Value][tDep] = exists
					}
				}
			}
			visited[name.Value] = true
			visiting[name.Value] = false

			sorted = append(sorted, e)
		}
		return true
	}

	// Repeatedly visit the first unvisited unode until none are left
	for {
		progress := false
		for _, e := range intermediates {
			if !visited[e.key().Value] {
				if visit(e.key()) {
					progress = true
				}
				break
			}
		}
		if !progress {
			break
		}
	}

	allDependencies := map[string][]graphNode{}

	for nodeName, dependencySet := range transitiveDependencies {
		var dependencyList []graphNode
		for dependencyName := range dependencySet {
			dependencyList = append(dependencyList, intermediates[dependencyName])
		}
		allDependencies[nodeName] = dependencyList
	}

	return sorted, allDependencies, diags
}

func checkUniqueNode(intermediates map[string]graphNode, node graphNode) syntax.Diagnostics {
	var diags syntax.Diagnostics

	key := node.key()
	name := key.Value
	if name == PulumiVarName {
		return syntax.Diagnostics{ast.ExprError(key, fmt.Sprintf("%s %s uses the reserved name pulumi", node.valueKind(), name), "")}
	}

	if other, found := intermediates[name]; found {
		if node.valueKind() == other.valueKind() {
			diags.Extend(ast.ExprError(key, fmt.Sprintf("found duplicate %s %s", node.valueKind(), name), ""))
		} else {
			diags.Extend(ast.ExprError(key, fmt.Sprintf("%s %s cannot have the same name as %s %s", node.valueKind(), name, other.valueKind(), name), ""))
		}
		return diags
	}
	return diags
}
