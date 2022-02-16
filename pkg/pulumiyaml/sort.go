// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

//see: https://github.com/BurntSushi/go-sumtype
//go-sumtype:decl IntermediateSymbol

type IntermediateSymbol interface {
	valueKind() string
	key() *ast.StringExpr
}

type intermediateResource struct {
	*ast.ResourcesMapEntry
}

func (e intermediateResource) valueKind() string {
	return "resource"
}

func (e intermediateResource) key() *ast.StringExpr {
	return e.Key
}

type intermediateVariable struct {
	*ast.VariablesMapEntry
}

func (e intermediateVariable) valueKind() string {
	return "variable"
}

func (e intermediateVariable) key() *ast.StringExpr {
	return e.Key
}

type intermediateConfig struct {
	*ast.ConfigMapEntry
}

func (e intermediateConfig) valueKind() string {
	return "config"
}

func (e intermediateConfig) key() *ast.StringExpr {
	return e.Key
}

func topologicallySortedResources(t *ast.TemplateDecl) ([]IntermediateSymbol, syntax.Diagnostics) {
	if t.Resources == nil {
		return nil, nil
	}

	var diags syntax.Diagnostics

	var sorted []IntermediateSymbol // will hold the sorted vertices.
	visiting := map[string]bool{}   // temporary entries to detect cycles.
	visited := map[string]bool{}    // entries to avoid visiting the same node twice.

	// Precompute dependencies for each resource
	intermediates := map[string]IntermediateSymbol{}
	dependencies := map[string][]*ast.StringExpr{}
	for _, kvp := range t.Resources.Entries {
		rname, r := kvp.Key.Value, kvp.Value
		if _, has := intermediates[rname]; has {
			diags.Extend(ast.ExprError(kvp.Key, fmt.Sprintf("duplicate resource name '%s'", rname), ""))
			continue
		}
		temp := kvp
		//nolint:govet // safety: we control this composite type
		intermediates[rname] = intermediateResource{&temp}
		dependencies[rname] = GetResourceDependencies(r)
	}
	if t.Configuration != nil {
		for _, kvp := range t.Configuration.Entries {
			cname := kvp.Key.Value
			// Prevent aliasing, see: http://blogs.msdn.com/b/ericlippert/archive/tags/closures/
			temp := kvp
			//nolint:govet // safety: we control this composite type
			intermediates[cname] = intermediateConfig{&temp}
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
			intermediates[vname] = intermediateVariable{&temp}
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

			switch e := e.(type) {
			case intermediateResource:
				sorted = append(sorted, &e)
			case intermediateVariable:
				sorted = append(sorted, &e)
			case intermediateConfig:
				sorted = append(sorted, &e)
			}
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
