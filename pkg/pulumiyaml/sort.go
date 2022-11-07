// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/config"
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

type configNode interface {
	graphNode
	value() interface{}
}

type configNodeYaml ast.ConfigMapEntry

func (e configNodeYaml) valueKind() string {
	return "config"
}

func (e configNodeYaml) key() *ast.StringExpr {
	return e.Key
}

func (e configNodeYaml) value() interface{} {
	return e.Value
}

type configNodeEnv struct {
	Key   string
	Value interface{}
	Type  config.Type
}

func (e configNodeEnv) valueKind() string {
	return "configEnv"
}

func (e configNodeEnv) key() *ast.StringExpr {
	return ast.String(e.Key)
}

func (e configNodeEnv) value() interface{} {
	return e.Value
}

type missingNode struct {
	name *ast.StringExpr
}

func (e missingNode) key() *ast.StringExpr {
	return e.name
}

func (missingNode) valueKind() string {
	return "missing node"
}

func topologicallySortedResources(t *ast.TemplateDecl, externalConfig []configNode) ([]graphNode, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	var sorted []graphNode        // will hold the sorted vertices.
	visiting := map[string]bool{} // temporary entries to detect cycles.
	visited := map[string]bool{}  // entries to avoid visiting the same node twice.

	// Precompute dependencies for each resource
	intermediates := map[string]graphNode{}
	// The list of keys to intermediates in the order they were first added.
	sortedIntermediatesKeys := []string{}

	// Add a new node to intermediates.
	addIntermediate := func(key string, node graphNode) {
		_, duplicate := intermediates[key]
		intermediates[key] = node
		if !duplicate {
			sortedIntermediatesKeys = append(sortedIntermediatesKeys, key)
		}
	}

	// A list of graphNodes from intermediates in the order they were inserted in.
	// This ensures iteration order is deterministic.
	intermediateValues := func() []graphNode {
		sorted := make([]graphNode, len(sortedIntermediatesKeys))
		for i, k := range sortedIntermediatesKeys {
			sorted[i] = intermediates[k]
		}
		return sorted
	}

	dependencies := map[string][]*ast.StringExpr{}

	templateConfig := make([]configNode, len(t.Configuration.Entries))
	for i, kvp := range t.Configuration.Entries {
		templateConfig[i] = configNode(configNodeYaml(kvp))
	}
	for _, node := range append(templateConfig, externalConfig...) {
		cname := node.key().Value
		cdiags := checkUniqueNode(intermediates, node)
		diags = append(diags, cdiags...)

		if !cdiags.HasErrors() {
			addIntermediate(cname, node)
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
			addIntermediate(rname, node)
			dependencies[rname] = GetResourceDependencies(r)
		}
	}
	for _, kvp := range t.Variables.Entries {
		vname := kvp.Key.Value
		node := variableNode(kvp)

		cdiags := checkUniqueNode(intermediates, node)
		diags = append(diags, cdiags...)

		if !cdiags.HasErrors() {
			addIntermediate(vname, node)
			dependencies[vname] = GetVariableDependencies(kvp)
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}

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
			diags.Extend(ast.ExprError(name, fmt.Sprintf("resource %q not found", name.Value), ""))
			e = missingNode{name}
			addIntermediate(name.Value, e)
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
				if mname.Value == PulumiVarName {
					continue
				}
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
		for _, e := range intermediateValues() {
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
	if name == PulumiVarName {
		return syntax.Diagnostics{ast.ExprError(key, fmt.Sprintf("%s %s uses the reserved name pulumi", node.valueKind(), name), "")}
	}

	if other, found := intermediates[name]; found {
		// if duplicate key from config/ configuration, do not warn about using configuration again
		if isConfigNodeEnv(node) || isConfigNodeEnv(other) {
			return diags
		}
		if node.valueKind() == other.valueKind() {
			diags.Extend(ast.ExprError(key, fmt.Sprintf("found duplicate %s %s", node.valueKind(), name), ""))
		} else {
			diags.Extend(ast.ExprError(key, fmt.Sprintf("%s %s cannot have the same name as %s %s", node.valueKind(), name, other.valueKind(), name), ""))
		}
		return diags
	}
	return diags
}

func isConfigNodeEnv(n graphNode) bool {
	_, ok := n.(configNodeEnv)
	return ok
}
