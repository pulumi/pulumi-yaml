// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"

	syn "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Generate a serializable YAML template.
func GenerateProgram(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	g := generator{
		invokeResults: map[*model.FunctionCallExpression]*InvokeCall{},
	}

	g.PrepareForInvokes(program)

	for _, n := range program.Nodes {
		g.genNode(n)
	}

	if g.diags.HasErrors() {
		return nil, nil, g.diags
	}

	w := &bytes.Buffer{}

	out := g.UnifyOutput()
	diags := encoding.EncodeYAML(yaml.NewEncoder(w), out)

	var err error
	if diags.HasErrors() {
		err = diags
	}

	if err != nil {
		return nil, nil, err
	}
	return map[string][]byte{"Main.yaml": w.Bytes()}, g.diags, err
}

type generator struct {
	diags hcl.Diagnostics
	// Because we need a return statement, we might need to call a invoke multiple times.
	invokeResults map[*model.FunctionCallExpression]*InvokeCall

	// These values can be assembled into a template
	config    []syn.ObjectPropertyDef
	resources []syn.ObjectPropertyDef
	variables []syn.ObjectPropertyDef
	outputs   []syn.ObjectPropertyDef
}

func (g *generator) UnifyOutput() syn.Node {
	entries := []syn.ObjectPropertyDef{}
	if len(g.config) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("config"), syn.Object(g.config...)))
	}
	if len(g.resources) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("resources"), syn.Object(g.resources...)))
	}
	if len(g.variables) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("variables"), syn.Object(g.variables...)))
	}
	if len(g.outputs) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("outputs"), syn.Object(g.outputs...)))
	}
	return syn.Object(entries...)
}

type InvokeCall struct {
	name       string
	usedValues codegen.StringSet
}

// Maps names to known invoke usages.
func (g *generator) InvokeNameMap() map[string]*InvokeCall {
	m := map[string]*InvokeCall{}
	for _, v := range g.invokeResults {
		m[v.name] = v
	}
	return m
}

// Prepares both the program and the generator for invokes.
// For the program:
//   Invokes are rewritten to handle that a return type must be used.
// For the generator:
//   We memoize what invokes are used so we can correctly give the Return value.
func (g *generator) PrepareForInvokes(program *pcl.Program) {
	for _, n := range program.Nodes {
		switch t := n.(type) {
		case *pcl.LocalVariable:
			if fn, ok := t.Definition.Value.(*model.FunctionCallExpression); ok && fn.Name == pcl.Invoke {
				// An empty list is different then nil
				g.invokeResults[fn] = &InvokeCall{t.Name(), codegen.NewStringSet()}
			}
		case *pcl.ConfigVariable:

		}
		// Pre should collect a list of references to invokes
		calls := g.InvokeNameMap()
		onTraversal := func(f func(*model.ScopeTraversalExpression) *model.ScopeTraversalExpression) func(n model.Expression) (model.Expression, hcl.Diagnostics) {
			return func(n model.Expression) (model.Expression, hcl.Diagnostics) {
				if t, ok := n.(*model.ScopeTraversalExpression); ok {
					return f(t), nil
				}
				return n, nil
			}
		}
		pre := onTraversal(func(t *model.ScopeTraversalExpression) *model.ScopeTraversalExpression {
			if call, ok := calls[t.RootName]; ok {
				path := strings.Split(g.expr(t).(*syn.StringNode).Value(), ".")
				contract.Assertf(len(path) > 1,
					"Invokes must use a field. They cannot just use the result of the invoke")
				// So note the referenced field
				immediateField := path[1]
				contract.Assertf(immediateField != "", "Empty path on %.v", t)
				call.usedValues.Add(path[1])
			}
			return t
		})
		// Post rewrites invoke variable access:
		// If only a single variable was accessed, just use the original name
		// If multiple variables were accessed, create a new named variable for each of them.
		post := onTraversal(func(t *model.ScopeTraversalExpression) *model.ScopeTraversalExpression {
			// This is an invoke
			if call, ok := calls[t.RootName]; ok {
				// Keep the root, but remove the first term of the traversal
				if len(call.usedValues) > 1 {
					panic("Multiple function calls unimplemented")
				}
				t.Parts = t.Parts[1:]
				t.Traversal = t.Traversal[1:]
				// remove the second term, which is handled by the return statement
			}
			return t
		})
		diags := n.VisitExpressions(pre, post)
		g.diags = g.diags.Extend(diags)
	}
}

type yamlLimitationKind string

var (
	Splat    yamlLimitationKind = "splat"
	ToJSON   yamlLimitationKind = "toJSON"
	toBase64 yamlLimitationKind = "toBase64"
)

func (y yamlLimitationKind) Summary() string {
	return fmt.Sprintf("Failed to generate YAML program. Missing %s", string(y))
}

func (g *generator) yamlLimitation(kind yamlLimitationKind) {
	g.diags = g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  kind.Summary(),
	})
}

func (g *generator) missingSchema() {
	g.diags = g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Could not get schema. This might lead to inacurate generation",
	})
}

func (g *generator) genNode(n pcl.Node) {
	switch n := n.(type) {
	case *pcl.Resource:
		g.genResource(n)
	case *pcl.ConfigVariable:
		g.genConfigVariable(n)
	case *pcl.LocalVariable:
		g.genLocalVariable(n)
	case *pcl.OutputVariable:
		g.genOutputVariable(n)
	default:
		panic("Not implemented yet")
	}
}

func unquoteInterpolation(n syn.Node) *syn.StringNode {
	s, ok := n.(*syn.StringNode)
	contract.Assert(ok)
	v := strings.TrimPrefix(s.Value(), "${")
	v = strings.TrimSuffix(v, "}")
	return syn.String(v)
}

func mustCoerceBoolean(n syn.Node) *syn.BooleanNode {
	switch n := n.(type) {
	case *syn.BooleanNode:
		return n
	case *syn.StringNode:
		return syn.Boolean(n.Value() == "true")
	default:
		panic(fmt.Sprintf("Could not coerce type %[1]T to a boolean: '%[1]#v'", n))
	}
}

func (g *generator) genResource(n *pcl.Resource) {
	rOpts := []syn.ObjectPropertyDef{}
	if opts := n.Options; opts != nil {
		if opts.Provider != nil {
			rOpts = append(rOpts, syn.ObjectProperty(syn.String("provider"),
				unquoteInterpolation(g.expr(opts.Provider))))
		}
		if opts.Parent != nil {
			rOpts = append(rOpts, syn.ObjectProperty(syn.String("provider"),
				unquoteInterpolation(g.expr(opts.Parent))))
		}
		if opts.DependsOn != nil {
			elems := g.expr(opts.DependsOn)
			_, ok := elems.(*syn.ListNode)
			contract.Assert(ok)
			rOpts = append(rOpts, syn.ObjectProperty(syn.String("dependson"), elems))
		}
		if opts.IgnoreChanges != nil {
			elems := g.expr(opts.IgnoreChanges).(*syn.ListNode)
			ignoreChanges := make([]syn.Node, elems.Len())
			for i := range ignoreChanges {
				ignoreChanges[i] = unquoteInterpolation(elems.Index(i))
			}
			list := syn.ListSyntax(elems.Syntax(), ignoreChanges...)
			rOpts = append(rOpts, syn.ObjectProperty(syn.String("ignorechanges"), list))
		}
		if opts.Protect != nil {
			expr := mustCoerceBoolean(g.expr(opts.Protect))
			rOpts = append(rOpts, syn.ObjectProperty(syn.String("protect"), expr))
		}
	}
	properties := make([]syn.ObjectPropertyDef, len(n.Inputs))
	var additionalSecrets []*syn.StringNode
	for i, input := range n.Inputs {
		value := input.Value
		if f, ok := value.(*model.FunctionCallExpression); ok && f.Name == "secret" {
			contract.Assert(len(f.Args) == 1)
			additionalSecrets = append(additionalSecrets, syn.String(input.Name))
			value = f.Args[0]
		}
		v := g.expr(value)
		properties[i] = syn.ObjectProperty(syn.String(input.Name), v)
	}
	if n.Schema == nil {
		g.missingSchema()
		n.Schema = &schema.Resource{}
	}
	entries := []syn.ObjectPropertyDef{
		syn.ObjectProperty(syn.String("type"), syn.String(n.Token)),
	}
	if n.Schema.IsComponent {
		entries = append(entries, syn.ObjectProperty(syn.String("component"), syn.Boolean(true)))
	}
	if len(properties) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("properties"), syn.Object(properties...)))
	}
	if len(rOpts) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("options"), syn.Object(rOpts...)))
	}
	r := syn.Object(entries...)

	g.resources = append(g.resources, syn.ObjectProperty(syn.String(n.Name()), r))
}

func (g *generator) genOutputVariable(n *pcl.OutputVariable) {
	k := syn.String(n.Name())
	v := g.expr(n.Value)
	g.outputs = append(g.outputs, syn.ObjectProperty(k, v))
}

func (g *generator) expr(e model.Expression) syn.Node {
	switch e := e.(type) {
	case *model.LiteralValueExpression:
		t := e.Value.Type()
		switch {
		case t.Equals(cty.NilType) || e.Value.IsNull():
			return syn.Null()
		case t.Equals(cty.String):
			v := e.Value.AsString()
			return syn.String(v)
		case t.Equals(cty.Number):
			v := e.Value.AsBigFloat()
			f, _ := v.Float64()
			return syn.Number(f)
		case t.Equals(cty.Bool):
			return syn.Boolean(e.Value.True())
		default:
			contract.Failf("Unexpected LiteralValueExpression (%[1]v): %[1]v", e.Type(), e)
			panic(nil)
		}

	case *model.FunctionCallExpression:
		return g.function(e)
	case *model.RelativeTraversalExpression:
		// This generally means a lookup scoped to a for loop. Since we don't do
		// those, we don't process RelativeTraversalExpressions.
		g.yamlLimitation("RelativeTraversalExpression")
		return syn.String("Unimplemented RelativeTraversalExpression")
	case *model.ScopeTraversalExpression:
		s := e.RootName
		for _, t := range e.Traversal.SimpleSplit().Rel {
			var key cty.Value
			switch t := t.(type) {
			case hcl.TraverseAttr:
				key = cty.StringVal(t.Name)
			case hcl.TraverseIndex:
				key = t.Key
			default:
				contract.Failf("Unexpected traverser of type %T: '%v'", t, t.SourceRange())
			}

			switch key.Type() {
			case cty.String:
				keyVal := key.AsString()
				s = fmt.Sprintf("%s.%s", s, keyVal)
			case cty.Number:
				idx, _ := key.AsBigFloat().Int64()
				s = fmt.Sprintf("%s[%d]", s, idx)
			default:
				keyExpr := &model.LiteralValueExpression{Value: key}
				diags := keyExpr.Typecheck(false)
				contract.Ignore(diags)

				s = fmt.Sprintf("%s[%v]", s, keyExpr)
			}
		}
		s = fmt.Sprintf("${%s}", s)
		return syn.String(s)

	case *model.TemplateExpression:
		useJoin := false
		nodes := []syn.Node{}
		for _, expr := range e.Parts {
			n := g.expr(expr)
			nodes = append(nodes, n)
			if _, ok := n.(*syn.StringNode); !ok {
				useJoin = true
			}
		}

		// Inline implies we can construct the string directly, using string interpolation for traversals.
		// Not inline means we need to use a Fn::Join statement.
		if useJoin {
			contract.Failf("Non-inline expressions are not implemented yet")
			panic(nil)
		}

		s := ""
		for _, expr := range e.Parts {
			// The inline check ensures that the cast is valid.
			s += g.expr(expr).(*syn.StringNode).Value()
		}
		return syn.String(s)

	case *model.TupleConsExpression:
		ls := make([]syn.Node, len(e.Expressions))
		for i, e := range e.Expressions {
			ls[i] = g.expr(e)
		}
		return syn.List(ls...)

	case *model.ObjectConsExpression:
		entries := make([]syn.ObjectPropertyDef, len(e.Items))
		for i, e := range e.Items {
			key := g.expr(e.Key).(*syn.StringNode)
			value := g.expr(e.Value)
			entries[i] = syn.ObjectProperty(key, value)
		}
		return syn.Object(entries...)

	case *model.SplatExpression:
		g.yamlLimitation(Splat)
		return syn.String("Splat not implemented")
	default:
		contract.Failf("Unimplimented: %[1]T. Needed for %[1]v", e)
		panic(nil)
	}
}

func (g *generator) genConfigVariable(n *pcl.ConfigVariable) {
	entries := []syn.ObjectPropertyDef{
		syn.ObjectProperty(syn.String("type"), syn.String(n.Type().String())),
	}
	if n.DefaultValue != nil {
		prop := syn.ObjectProperty(syn.String("default"), g.expr(n.DefaultValue))
		entries = append(entries, prop)
	}

	k := syn.String(n.Name())
	v := syn.Object(entries...)
	g.config = append(g.config, syn.ObjectProperty(k, v))
}

func (g *generator) genLocalVariable(n *pcl.LocalVariable) {
	k := syn.String(n.Name())
	v := g.expr(n.Definition.Value)
	entry := syn.ObjectProperty(k, v)
	g.variables = append(g.variables, entry)
}

func (g *generator) function(f *model.FunctionCallExpression) *syn.ObjectNode {
	fn := func(name string, body syn.Node) *syn.ObjectNode {
		entry := syn.ObjectProperty(syn.String("Fn::"+name), body)
		return syn.Object(entry)
	}
	switch f.Name {
	case pcl.Invoke:
		return fn("Invoke", g.MustInvoke(f))
	case "fileAsset":
		return fn("Asset", syn.Object(
			syn.ObjectProperty(syn.String("File"), g.expr(f.Args[0])),
		))
	case "join":
		args := make([]syn.Node, len(f.Args))
		for i, arg := range f.Args {
			args[i] = g.expr(arg)
		}
		return fn("Join", syn.List(args...))
	case "toJSON":
		g.yamlLimitation(ToJSON)
		return fn("toJSON", syn.Null())
	case "toBase64":
		g.yamlLimitation(toBase64)
		return fn("toBase64", syn.Null())
	default:
		panic(fmt.Sprintf("function '%s' has not been implemented", f.Name))
	}
}

type Invoke struct {
	Function  string      `yaml:"Function" json:"Function"`
	Arguments interface{} `yaml:"Arguments,omitempty" json:"Arguments,omitempty"`
	Return    string      `yaml:"Return" json:"Return"`
}

func (g *generator) MustInvoke(f *model.FunctionCallExpression) *syn.ObjectNode {
	contract.Assert(f.Name == pcl.Invoke)
	contract.Assert(len(f.Args) > 0)
	name := g.expr(f.Args[0])
	var arguments syn.Node
	if len(f.Args) > 1 {
		_, ok := f.Args[1].(*model.ObjectConsExpression)
		contract.Assert(ok)
		arguments = g.expr(f.Args[1])
	}

	// Calculate the return value
	fnInfo := g.invokeResults[f]
	contract.Assertf(len(fnInfo.usedValues) > 0,
		"Invoke assigned to %s has no used values. Dumping known invokes: %#v",
		fnInfo.name, g.invokeResults)
	var retValue *syn.StringNode
	if len(fnInfo.usedValues) == 1 {
		retValue = syn.String(fnInfo.usedValues.SortedValues()[0])
	} else {
		panic("unimplemented")
	}
	return syn.Object(
		syn.ObjectProperty(syn.String("Function"), name),
		syn.ObjectProperty(syn.String("Arguments"), arguments),
		syn.ObjectProperty(syn.String("Return"), retValue),
	)
}
