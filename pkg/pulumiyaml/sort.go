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

func topologicallySortedResources(t *ast.TemplateDecl) ([]graphNode, syntax.Diagnostics) {
	if t.Resources == nil {
		return nil, nil
	}

	var diags syntax.Diagnostics

	var sorted []graphNode        // will hold the sorted vertices.
	visiting := map[string]bool{} // temporary entries to detect cycles.
	visited := map[string]bool{}  // entries to avoid visiting the same node twice.

	// Precompute dependencies for each resource
	intermediates := map[string]graphNode{}
	dependencies := map[string][]*ast.StringExpr{}
	if t.Configuration != nil {
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
	if t.Variables != nil {
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
	}

	if diags.HasErrors() {
		return nil, diags
	}

	// Depth-first visit each node
	var visit func(name *ast.StringExpr) bool
	visit = func(name *ast.StringExpr) bool {
		e, ok := intermediates[name.Value]
		if !ok {
			diags.Extend(ast.ExprError(name, fmt.Sprintf("dependency %s not found", name.Value), ""))
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
			visiting[name.Value] = true
			for _, mname := range dependencies[name.Value] {
				if !visit(mname) {
					return false
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
	return sorted, diags
}

func checkUniqueNode(intermediates map[string]graphNode, node graphNode) syntax.Diagnostics {
	var diags syntax.Diagnostics

	key := node.key()
	name := key.Value
	if name == "pulumi" {
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
