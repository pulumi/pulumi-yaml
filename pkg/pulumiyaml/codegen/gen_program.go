// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ettle/strcase"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	syn "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Generate a serializable YAML template.
func GenerateProgram(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	g := generator{}

	for _, n := range program.Nodes {
		g.genNode(n)
	}

	if g.diags.HasErrors() {
		return nil, g.diags, nil
	}

	w := &bytes.Buffer{}

	out := g.UnifyOutput()
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	diags := encoding.EncodeYAML(encoder, out)

	var err error
	if diags.HasErrors() {
		err = diags
	}

	return map[string][]byte{"Main.yaml": w.Bytes()}, g.diags, err
}

type generator struct {
	diags hcl.Diagnostics

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

// A user-facing yaml error
type YAMLError struct {
	kind   string
	detail string
	rng    hcl.Range
}

func (l YAMLError) AppendTo(g *generator) {
	var summary string
	if l.kind != "" {
		summary = "Failed to generate YAML program: " + l.kind
	}
	g.diags = g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  summary,
		Detail:   l.detail,
		Subject:  &l.rng,
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

// genResourceOpts converts pcl.ResourceOptions to an YAML Object for use in genResource.
func (g *generator) genResourceOpts(opts *pcl.ResourceOptions) *syn.ObjectNode {
	if opts == nil {
		return nil
	}

	rOpts := []syn.ObjectPropertyDef{}
	if opts.Provider != nil {
		rOpts = append(rOpts, syn.ObjectProperty(syn.String("provider"),
			g.expr(opts.Provider)))
	}
	if opts.Parent != nil {
		rOpts = append(rOpts, syn.ObjectProperty(syn.String("provider"),
			g.expr(opts.Parent)))
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

	return syn.Object(rOpts...)
}

func (g *generator) genResource(n *pcl.Resource) {
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
	}

	entries := []syn.ObjectPropertyDef{
		g.TypeProperty(collapseToken(n.Token)),
	}
	if n.Schema != nil && n.Schema.IsComponent {
		entries = append(entries, syn.ObjectProperty(syn.String("component"), syn.Boolean(true)))
	}
	if len(properties) > 0 {
		entries = append(entries, syn.ObjectProperty(syn.String("properties"), syn.Object(properties...)))
	}
	if opts := g.genResourceOpts(n.Options); opts != nil {
		entries = append(entries, syn.ObjectProperty(syn.String("options"), opts))
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
	g        *generator
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

// isEscapedString checks is a string is properly escaped. This means that it
// starts and ends with a '"', and that any '"' in the middle of the string is
// itself escaped: '\"'.
//
// Valid:
// - "foobar"
// - "foo\"bar"
// - "foo\\\"bar"
// Invalid:
// - "foo
// - "foo"bar"
// - "foo\\"bar"
func isEscapedString(s string) bool {

	if !strings.HasPrefix(s, `"`) {
		return false
	}
	s = s[1:]
	if !strings.HasSuffix(s, `"`) {
		return false
	}
	s = strings.TrimSuffix(s, `"`)

	// We should encounter only escaped '"' at this point
	isEscaped := false
	for len(s) > 0 {
		switch s[0] {
		case '\\':
			isEscaped = !isEscaped
		case '"':
			if !isEscaped {
				return false
			}
			fallthrough
		default:
			isEscaped = false
		}
		s = s[1:]
	}
	return true
}

func (g *generator) Traversal(traversal hcl.Traversal) Traversal {

	var segments []TraversalSegment
	if !traversal.IsRelative() {
		traversal = traversal.SimpleSplit().Rel
	}
	for _, t := range traversal {
		segments = append(segments, g.TraversalSegment(t))
	}
	return Traversal{"", segments, g}
}

func (t Traversal) String() string {
	s := t.root
	for _, ts := range t.segments {
		if ts.omit {
			continue
		}
		s += fmt.Sprintf(ts.joinFmt, ts.segment)
	}
	if t.root == "" {
		return strings.TrimPrefix(s, ".")
	}
	return s
}

func (t Traversal) WithRoot(s string, hclRange *hcl.Range) Traversal {
	if checked := t.g.checkPropertyName(s, hclRange); checked != "" {
		s = checked
	}
	t.root = s
	return t
}

func (t Traversal) OmitFirst() Traversal {
	t.segments[0].omit = true
	return t
}

// checkPropertyName checks if a property name is valid. If invalid, an escaped
// quoted string is returned to be used as a property map access. Otherwise, the
// empty string is returned.
func (g *generator) checkPropertyName(n string, subject *hcl.Range) string {
	if !ast.PropertyNameRegexp.Match([]byte(n)) {
		g.diags = append(g.diags, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Invalid property name access",
			Detail:   fmt.Sprintf("'%s' is not a valid property name access. It has been changed to a quoted key access.", n),
			Subject:  subject,
		})
		return asEscapedString(n)
	}
	return ""
}

// checkPropertyKeyIndex checks if a property key is valid. If not valid, a
// valid property key string is returned. Otherwise the empty string is
// returned.
func (g *generator) checkPropertyKeyIndex(n string, subject *hcl.Range) string {
	if !isEscapedString(n) {
		g.diags = append(g.diags, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  "Invalid property key access",
			Detail:   fmt.Sprintf("'%s' is not a valid property key access. It has been appropriately quoted.", n),
			Subject:  subject,
		})

		return asEscapedString(n)
	}
	return ""
}

// asEscapedString returns s where all `"` in a string are escaped. It ensures
// that quotations are applied around the string.
//
// For example:
//     `foo` -> `"foo"`
//    `bar"` -> `"bar"`
// `foo"bar` -> `"foo\"bar"`
func asEscapedString(s string) string {
	s = strings.TrimSuffix(s, `"`)
	s = strings.TrimPrefix(s, `"`)
	var out strings.Builder
	out.WriteRune('"')
	var escaped bool
	for _, c := range s {
		switch c {
		case '\\':
			escaped = !escaped
		case '"':
			if !escaped {
				out.WriteRune('\\')
			}
			fallthrough
		default:
			escaped = false
		}
		out.WriteRune(c)
	}
	out.WriteRune('"')
	return out.String()
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
		if s := g.checkPropertyName(keyVal, t.SourceRange().Ptr()); s != "" {
			// We convert invalid property names to property accesses.
			return TraversalSegment{
				segment: s,
				joinFmt: "%s[%s]",
			}
		}
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
		segment := fmt.Sprintf("%v", keyExpr)
		if s := g.checkPropertyKeyIndex(segment, t.SourceRange().Ptr()); s != "" {
			segment = s
		}
		return TraversalSegment{
			segment: segment,
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
		// Direct use of a function
		if f, ok := e.Source.(*model.FunctionCallExpression); ok {
			// Invokes can process a return type
			if f.Name == pcl.Invoke {
				return g.MustInvoke(f, g.Traversal(e.Traversal).String())
			}
			// But normal functions cannot
			if len(e.Traversal) > 0 {
				YAMLError{
					kind:   "Traversal not allowed on function result",
					detail: "Cannot traverse the return value of Fn::" + f.Name,
					rng:    f.Syntax.Range(),
				}.AppendTo(g)
			}
			return g.function(f)
		}
		// This generally means a lookup scoped to a for loop. Since we don't do
		// those, we don't process this type of RelativeTraversalExpressions.
		YAMLError{
			kind: "Unsupported Expression",
			detail: "This use of a RelativeTraversalExpression is not supported in YAML.\n" +
				"YAML may not be expressive enough to support this expression.\n" +
				"It is also possible that the expression could be supported, but has not been implemented.",
			rng: e.Syntax.Range(),
		}.AppendTo(g)
		return syn.String("Unimplemented Expression")

	case *model.ScopeTraversalExpression:
		traversal := g.Traversal(e.Traversal).WithRoot(e.RootName, e.Tokens.Root.Range().Ptr())
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

		// useJoin implies we need to use "Fn::Join" to construct the desired
		// string.
		if useJoin {
			return wrapFn("Join", syn.List(syn.String(""), syn.List(nodes...)))
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
		YAMLError{
			kind: "Splat",
			detail: "A 'Splat' expression is equivalent in expressive power to a 'for loop', and is not representable in YAML.\n" +
				"If the values of the function are known, you can manually unroll the loop," +
				" otherwise it is necessary to switch to a more expressive language.",
			rng: e.Syntax.Range(),
		}.AppendTo(g)
		return syn.String("Splat not implemented")
	case *model.ForExpression:
		YAMLError{
			kind: "For",
			detail: "Pulumi YAML cannot represent for loops." +
				"If the values of the for loop are known, you can manually unroll the loop," +
				" otherwise it is necessary to switch to a more expressive language.",
			rng: e.Syntax.Range(),
		}.AppendTo(g)
		return syn.String("For not implemented")
	default:
		YAMLError{
			kind:   fmt.Sprintf("%T", e),
			detail: fmt.Sprintf("Unimplemented! Needed for %v", e),
			rng:    e.SyntaxNode().Range(),
		}.AppendTo(g)
		return syn.String("Unimplemented")
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

func (g *generator) function(f *model.FunctionCallExpression) syn.Node {
	getRange := func() hcl.Range {
		if s := f.Syntax; s != nil {
			return s.Range()
		}
		var rng hcl.Range
		return rng
	}
	switch f.Name {
	case pcl.Invoke:
		return g.MustInvoke(f, "")
	case "fileArchive", "remoteArchive", "assetArchive",
		"fileAsset", "stringAsset", "remoteAsset":
		return wrapFn(strcase.ToPascal(f.Name), g.expr(f.Args[0]))
	case "join":
		args := make([]syn.Node, len(f.Args))
		for i, arg := range f.Args {
			args[i] = g.expr(arg)
		}
		return wrapFn("Join", syn.List(args...))
	case "split":
		return wrapFn("Split", syn.List(g.expr(f.Args[1]), g.expr(f.Args[0])))
	case "toBase64":
		return wrapFn("ToBase64", g.expr(f.Args[0]))
	case "toJSON":
		return wrapFn("ToJSON", g.expr(f.Args[0]))
	case "element":
		args := make([]syn.Node, len(f.Args))
		for i, arg := range f.Args {
			args[i] = g.expr(arg)
		}
		return wrapFn("Select", syn.List(args[1], args[0]))
	case pcl.IntrinsicConvert:
		// We can't perform the convert, but it might happen automatically.
		// This works for enums, as well as number -> strings.
		if len(f.Args) > 0 {
			return g.expr(f.Args[0])
		}
		YAMLError{
			kind:   "Malformed Convert Intrinsic",
			detail: "Missing arguments",
			rng:    getRange(),
		}.AppendTo(g)
		return wrapFn(f.Name, syn.Null())
	default:
		YAMLError{
			kind:   "Unknown Function",
			detail: fmt.Sprintf("YAML does not support Fn::%s.", f.Name),
			rng:    getRange(),
		}.AppendTo(g)
		return wrapFn(f.Name, syn.Null())
	}
}

func wrapFn(name string, body syn.Node) *syn.ObjectNode {
	entry := syn.ObjectProperty(syn.String("Fn::"+name), body)
	return syn.Object(entry)
}

type Invoke struct {
	Function  string      `yaml:"Function" json:"Function"`
	Arguments interface{} `yaml:"Arguments,omitempty" json:"Arguments,omitempty"`
	Return    string      `yaml:"Return" json:"Return"`
}

func (g *generator) MustInvoke(f *model.FunctionCallExpression, ret string) *syn.ObjectNode {
	contract.Assert(f.Name == pcl.Invoke)
	contract.Assert(len(f.Args) > 0)
	name := collapseToken(g.expr(f.Args[0]).(*syn.StringNode).Value())

	var arguments syn.Node
	if len(f.Args) > 1 {
		_, ok := f.Args[1].(*model.ObjectConsExpression)
		contract.Assert(ok)
		arguments = g.expr(f.Args[1])
	}

	properties := []syn.ObjectPropertyDef{
		syn.ObjectProperty(syn.String("Function"), syn.String(name)),
		syn.ObjectProperty(syn.String("Arguments"), arguments),
	}

	// Calculate the return value
	if ret != "" {
		properties = append(properties,
			syn.ObjectProperty(
				syn.String("Return"),
				syn.String(ret),
			))
	}

	return wrapFn("Invoke", syn.Object(properties...))
}

func (g *generator) TypeProperty(s string) syn.ObjectPropertyDef {
	return syn.ObjectProperty(syn.String("type"), syn.String(s))
}

// collapseToken converts an exact token to a token more suitable for
// display. For example, it converts
//   fizz:index/buzz:Buzz => fizz:Buzz
//   fizz:mode/buzz:Buzz  => fizz:mode:Buzz
// 	 foo:index:Bar	      => foo:Bar
// 	 foo::Bar             => foo:Bar
// 	 fizz:mod:buzz        => fizz:mod:buzz
// collapseToken is a partial inverse of `(pulumiyaml.resourcePackage).ResolveResource`.
func collapseToken(token string) string {
	tokenParts := strings.Split(token, ":")

	if len(tokenParts) == 3 {
		title := func(s string) string {
			r := []rune(s)
			if len(r) == 0 {
				return ""
			}
			return strings.ToTitle(string(r[0])) + string(r[1:])
		}
		if mod := strings.Split(tokenParts[1], "/"); len(mod) == 2 && title(mod[1]) == tokenParts[2] {
			// aws:s3/bucket:Bucket => aws:s3:Bucket
			// We recourse to handle the case foo:index/bar:Bar => foo:index:Bar
			tokenParts = []string{tokenParts[0], mod[0], tokenParts[2]}
		}

		if tokenParts[1] == "index" || tokenParts[1] == "" {
			// foo:index:Bar => foo:Bar
			// or
			// foo::Bar => foo:Bar
			tokenParts = []string{tokenParts[0], tokenParts[2]}
		}
	}

	return strings.Join(tokenParts, ":")
}
