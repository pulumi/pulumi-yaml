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
		entries = append(entries, syn.ObjectProperty(syn.String("configuration"), syn.Object(g.config...)))
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
	usedValues TraversalList
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
				g.invokeResults[fn] = &InvokeCall{t.Name(), NewTraversalList()}
			}
		case *pcl.ConfigVariable:

		}
		calls := g.InvokeNameMap()
		onTraversal := func(f func(*model.ScopeTraversalExpression) *model.ScopeTraversalExpression) func(n model.Expression) (model.Expression, hcl.Diagnostics) {
			return func(n model.Expression) (model.Expression, hcl.Diagnostics) {
				if t, ok := n.(*model.ScopeTraversalExpression); ok {
					return f(t), nil
				}
				return n, nil
			}
		}

		// Pre should collect a list of references to invokes
		pre := onTraversal(func(t *model.ScopeTraversalExpression) *model.ScopeTraversalExpression {
			if call, ok := calls[t.RootName]; ok {
				// So note the referenced field
				traversal := g.Traversal(t)
				call.usedValues = AppendTraversal(call.usedValues, traversal)
			}
			return t
		})
		// Post rewrites invoke variable access if a single variable was accessed.
		post := onTraversal(func(t *model.ScopeTraversalExpression) *model.ScopeTraversalExpression {
			// This is an invoke, and it is only used once: rewrite it to use a
			// return statement
			if call, ok := calls[t.RootName]; ok && len(call.usedValues) == 1 {
				call.usedValues[0] = call.usedValues[0].OmitFirst()
			}
			return t
		})
		diags := n.VisitExpressions(pre, post)
		g.diags = g.diags.Extend(diags)
	}
}

func (g *generator) yamlLimitation(kind string) {
	g.diags = g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  kind,
		Detail:   fmt.Sprintf("Failed to generate YAML program. Missing function %s", kind),
	})
}

func (g *generator) missingSchema() {
	g.diags = g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Could not get schema. This might lead to inaccurate generation",
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
	typ := n.Token
	if n.Schema != nil && n.Schema.Token != "" {
		typ = n.Schema.Token
	}
	entries := []syn.ObjectPropertyDef{
		g.TypeProperty(typ),
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

// A segment of `Traversal`
type TraversalSegment struct {
	segment string
	joinFmt string

	// Omit this segment when displaying
	omit bool
}

// A `ScopedTraversalExpression` ready to display.
type Traversal struct {
	root     string
	segments []TraversalSegment
}

func (t Traversal) Equal(o Traversal) bool {
	if t.root != o.root || len(t.segments) != len(o.segments) {
		return false
	}
	for i := range t.segments {
		if t.segments[i].segment != o.segments[i].segment ||
			t.segments[i].joinFmt != o.segments[i].joinFmt {
			return false
		}
	}
	return true
}

type TraversalList = []Traversal

func NewTraversalList() TraversalList {
	return make([]Traversal, 0)
}

func AppendTraversal(tl TraversalList, t Traversal) TraversalList {
	for _, o := range tl {
		if t.Equal(o) {
			return tl
		}
	}
	return append(tl, t)
}

func (g *generator) Traversal(t *model.ScopeTraversalExpression) Traversal {

	var segments []TraversalSegment
	traversal := t.Traversal
	if !traversal.IsRelative() {
		traversal = traversal.SimpleSplit().Rel
	}
	for _, t := range traversal {
		segments = append(segments, g.TraversalSegment(t))
	}
	tr := Traversal{t.RootName, segments}

	// In theory, this is quite slow. Inserting n references would run in O(n^3)
	// time. In practice our examples are quite small, so it doesn't matter.
	for _, call := range g.invokeResults {
		for _, instance := range call.usedValues {
			if instance.Equal(tr) {
				return instance
			}
		}
	}

	return tr
}

func (t Traversal) String() string {
	s := t.root
	for _, ts := range t.segments {
		if ts.omit {
			continue
		}
		s += fmt.Sprintf(ts.joinFmt, ts.segment)
	}
	return s
}

func (t Traversal) OmitFirst() Traversal {
	t.segments[0].omit = true
	return t
}

func (g *generator) TraversalSegment(t hcl.Traverser) TraversalSegment {
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
		return TraversalSegment{
			segment: keyVal,
			joinFmt: ".%s",
		}
	case cty.Number:
		idx, _ := key.AsBigFloat().Int64()
		return TraversalSegment{
			segment: fmt.Sprintf("%d", idx),
			joinFmt: "[%s]",
		}
	default:
		keyExpr := &model.LiteralValueExpression{Value: key}
		diags := keyExpr.Typecheck(false)
		contract.Ignore(diags)
		return TraversalSegment{
			segment: fmt.Sprintf("%v", keyExpr),
			joinFmt: "%s[%s]",
		}
	}
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
		traversal := g.Traversal(e)
		s := fmt.Sprintf("${%s}", traversal)
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
		g.yamlLimitation("Splat")
		return syn.String("Splat not implemented")
	default:
		contract.Failf("Unimplimented: %[1]T. Needed for %[1]v", e)
		panic(nil)
	}
}

func (g *generator) genConfigVariable(n *pcl.ConfigVariable) {
	entries := []syn.ObjectPropertyDef{
		g.TypeProperty(n.Type().String()),
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
	case "toBase64":
		return fn("ToBase64", g.expr(f.Args[0]))
	case "toJSON":
		return fn("ToJSON", g.expr(f.Args[0]))
	case "filebase64", "readFile":
		g.yamlLimitation(f.Name)
		return fn(f.Name, syn.Null())
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
	if segments := strings.Split(name.(*syn.StringNode).Value(), ":"); len(segments) == 3 && !strings.Contains(segments[1], "/") {
		if segments[1] == "" {
			segments[1] = "index"
		}
		t := segments[2]
		if len(t) > 0 {
			t = strings.ToLower(t[:1]) + t[1:]
		}
		segments[1] = segments[1] + "/" + t
		name = syn.String(strings.Join(segments, ":"))
	}
	var arguments syn.Node
	if len(f.Args) > 1 {
		_, ok := f.Args[1].(*model.ObjectConsExpression)
		contract.Assert(ok)
		arguments = g.expr(f.Args[1])
	}

	properties := []syn.ObjectPropertyDef{
		syn.ObjectProperty(syn.String("Function"), name),
		syn.ObjectProperty(syn.String("Arguments"), arguments),
	}

	// Calculate the return value
	if fnInfo := g.invokeResults[f]; len(fnInfo.usedValues) == 1 {
		properties = append(properties,
			syn.ObjectProperty(
				syn.String("Return"),
				syn.String(fnInfo.usedValues[0].segments[0].segment),
			))
	}

	return syn.Object(properties...)
}

func (g *generator) TypeProperty(s string) syn.ObjectPropertyDef {
	return syn.ObjectProperty(syn.String("type"), syn.String(s))
}
