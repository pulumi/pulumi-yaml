// Copyright 2022, Pulumi Corporation.  All rights reserved.

package ast

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

var fnInvokeRegex = regexp.MustCompile("fn::[^:]+:[^:]+(:[^:]+)?$")

// Expr represents a Pulumi YAML expression. Expressions may be literals, interpolated strings, symbols, or builtin
// functions.
type Expr interface {
	Syntax() syntax.Node

	isExpr()
}

type exprNode struct {
	syntax syntax.Node
}

func expr(node syntax.Node) exprNode {
	return exprNode{syntax: node}
}

func (*exprNode) isExpr() {}

func (x *exprNode) Syntax() syntax.Node {
	if x == nil {
		return nil
	}
	return x.syntax
}

// ExprError creates an error-level diagnostic associated with the given expression. If the expression is non-nil and
// has an underlying syntax node, the error will cover the underlying textual range.
func ExprError(expr Expr, summary, detail string) *syntax.Diagnostic {
	var rng *hcl.Range
	if expr != nil {
		if syntax := expr.Syntax(); syntax != nil {
			rng = syntax.Syntax().Range()
		}
	}
	return syntax.Error(rng, summary, detail)
}

// A NullExpr represents a null literal.
type NullExpr struct {
	exprNode
}

// NullSyntax creates a new null literal expression with associated syntax.
func NullSyntax(node *syntax.NullNode) *NullExpr {
	return &NullExpr{exprNode: expr(node)}
}

// Null creates a new null literal expression.
func Null() *NullExpr {
	return &NullExpr{}
}

// A BooleanExpr represents a boolean literal.
type BooleanExpr struct {
	exprNode

	Value bool
}

// BooleanSyntax creates a new boolean literal expression with the given value and associated syntax.
func BooleanSyntax(node *syntax.BooleanNode) *BooleanExpr {
	return &BooleanExpr{exprNode: expr(node), Value: node.Value()}
}

// Boolean creates a new boolean literal expression with the given value.
func Boolean(value bool) *BooleanExpr {
	return &BooleanExpr{Value: value}
}

// A NumberExpr represents a number literal.
type NumberExpr struct {
	exprNode

	Value float64
}

// NumberSyntax creates a new number literal expression with the given value and associated syntax.
func NumberSyntax(node *syntax.NumberNode) *NumberExpr {
	return &NumberExpr{exprNode: expr(node), Value: node.Value()}
}

// Number creates a new number literal expression with the given value.
func Number(value float64) *NumberExpr {
	return &NumberExpr{Value: value}
}

// A StringExpr represents a string literal.
type StringExpr struct {
	exprNode

	Value string
}

// GetValue returns the expression's value. If the receiver is null, GetValue returns the empty string.
func (x *StringExpr) GetValue() string {
	if x == nil {
		return ""
	}
	return x.Value
}

// StringSyntax creates a new string literal expression with the given value and associated syntax.
func StringSyntax(node *syntax.StringNode) *StringExpr {
	return &StringExpr{exprNode: expr(node), Value: node.Value()}
}

// StringSyntaxValue creates a new string literal expression with the given syntax and value.
func StringSyntaxValue(node *syntax.StringNode, value string) *StringExpr {
	return &StringExpr{exprNode: expr(node), Value: value}
}

// String creates a new string literal expression with the given value.
func String(value string) *StringExpr {
	return &StringExpr{Value: value}
}

// An InterpolateExpr represents an interpolated string.
//
// Interpolated strings are represented syntactically as strings of the form "some text with ${property.accesses}".
// During evaluation, each access replaced with its evaluated value coerced to a string.
//
// In order to allow convenient access to object properties without string coercion, a string of the form
// "${property.access}" is parsed as a symbol rather than an interpolated string.
type InterpolateExpr struct {
	exprNode

	Parts []Interpolation
}

func (n *InterpolateExpr) String() string {
	var str strings.Builder
	for _, p := range n.Parts {
		// un-escape the string back to its original form
		// this is necessary because the parser will escape the string
		// so when we print it back out as string, we need to un-escape it
		str.WriteString(strings.ReplaceAll(p.Text, "$", "$$"))
		if p.Value != nil {
			fmt.Fprintf(&str, "${%v}", p.Value)
		}
	}
	return str.String()
}

// InterpolateSyntax creates a new interpolated string expression with associated syntax by parsing the given input
// string literal.
func InterpolateSyntax(node *syntax.StringNode) (*InterpolateExpr, syntax.Diagnostics) {
	parts, diags := parseInterpolate(node, node.Value())
	if diags.HasErrors() {
		return nil, diags
	}

	for _, part := range parts {
		if part.Value != nil && len(part.Value.Accessors) == 0 {
			diags.Extend(syntax.NodeError(node, "Property access expressions cannot be empty", ""))
		}
	}

	return &InterpolateExpr{
		exprNode: expr(node),
		Parts:    parts,
	}, diags
}

// Interpolate creates a new interpolated string expression by parsing the given input string.
func Interpolate(value string) (*InterpolateExpr, syntax.Diagnostics) {
	return InterpolateSyntax(syntax.String(value))
}

func VariableSubstitution(value string) (*SymbolExpr, syntax.Diagnostics) {
	value = "${" + value + "}"
	node := syntax.String(value)
	interpolate, diags := InterpolateSyntax(node)
	if diags.HasErrors() {
		return nil, diags
	}

	if interpolate != nil && len(interpolate.Parts) == 1 && interpolate.Parts[0].Text == "" {
		return &SymbolExpr{
			exprNode: expr(node),
			Property: interpolate.Parts[0].Value,
		}, diags
	}

	return nil, syntax.Diagnostics{syntax.NodeError(node, "Must be a valid substitution, e.g.: like ${resourceName}", "")}
}

// MustInterpolate creates a new interpolated string expression and panics if parsing fails.
func MustInterpolate(value string) *InterpolateExpr {
	x, diags := Interpolate(value)
	if diags.HasErrors() {
		panic(diags)
	}
	return x
}

// A SymbolExpr represents a symbol: a reference to a resource or config property.
//
// Symbol expressions are represented as strings of the form "${resource.property}".
type SymbolExpr struct {
	exprNode

	Property *PropertyAccess
}

func (n *SymbolExpr) String() string {
	return fmt.Sprintf("${%v}", n.Property)
}

// A ListExpr represents a list of expressions.
type ListExpr struct {
	exprNode

	Elements []Expr
}

// ListSyntax creates a new list expression with the given elements and associated syntax.
func ListSyntax(node *syntax.ListNode, elements ...Expr) *ListExpr {
	return &ListExpr{
		exprNode: expr(node),
		Elements: elements,
	}
}

// List creates a new list expression with the given elements.
func List(elements ...Expr) *ListExpr {
	return ListSyntax(syntax.List(), elements...)
}

// An ObjectExpr represents an object.
type ObjectExpr struct {
	exprNode

	Entries []ObjectProperty
}

// An ObjectProperty represents an object property. Key must be a string, interpolated string, or symbol.
type ObjectProperty struct {
	syntax syntax.ObjectPropertyDef
	Key    Expr
	Value  Expr
}

// ObjectSyntax creates a new object expression with the given properties and associated syntax.
func ObjectSyntax(node *syntax.ObjectNode, entries ...ObjectProperty) *ObjectExpr {
	return &ObjectExpr{
		exprNode: expr(node),
		Entries:  entries,
	}
}

// Object creates a new object expression with the given properties.
func Object(entries ...ObjectProperty) *ObjectExpr {
	return ObjectSyntax(syntax.ObjectSyntax(syntax.NoSyntax), entries...)
}

// ParseExpr parses an expression from the given syntax tree.
//
// The syntax tree is parsed using the following rules:
//
//   - *syntax.{Null,Boolean,Number}Node is parsed as a *{Null,Boolean,Number}Expr.
//   - *syntax.ListNode is parsed as a *ListExpr.
//   - *syntax.StringNode is parsed as an *InterpolateExpr, a *SymbolExpr, or a *StringExpr. The node's literal is first
//     parsed as an interpolated string. If the result contains a single property access with no surrounding text, (i.e.
//     the string is of the form "${resource.property}", it is treated as a symbol. If the result contains no property
//     accesses, it is treated as a string literal. Otherwise, it it treated as an interpolated string.
//   - *syntax.ObjectNode is parses as either an *ObjectExpr or a BuiltinExpr. If the object contains a single key and
//     that key names a builtin function ("fn::invoke", "fn::join", "fn::select",
//     "fn::*Asset", "fn::*Archive", or "fn::stackReference"), then the object is parsed as the corresponding BuiltinExpr.
//     Otherwise, the object is parsed as a *syntax.ObjectNode.
func ParseExpr(node syntax.Node) (Expr, syntax.Diagnostics) {
	switch node := node.(type) {
	case *syntax.NullNode:
		return NullSyntax(node), nil
	case *syntax.BooleanNode:
		return BooleanSyntax(node), nil
	case *syntax.NumberNode:
		return NumberSyntax(node), nil
	case *syntax.StringNode:
		interpolate, diags := InterpolateSyntax(node)

		if interpolate != nil {
			switch len(interpolate.Parts) {
			case 0:
				return StringSyntax(node), diags
			case 1:
				switch {
				case interpolate.Parts[0].Value == nil:
					return StringSyntaxValue(node, interpolate.Parts[0].Text), diags
				case interpolate.Parts[0].Text == "":
					return &SymbolExpr{
						exprNode: expr(node),
						Property: interpolate.Parts[0].Value,
					}, diags
				}
			}
		}

		return interpolate, diags
	case *syntax.ListNode:
		var diags syntax.Diagnostics

		elements := make([]Expr, node.Len())
		for i := range elements {
			x, xdiags := ParseExpr(node.Index(i))
			diags.Extend(xdiags...)
			elements[i] = x
		}
		return ListSyntax(node, elements...), diags
	case *syntax.ObjectNode:

		var diags syntax.Diagnostics

		x, fnDiags, ok := tryParseFunction(node)
		if ok {
			return x, fnDiags
		}
		diags.Extend(fnDiags...)

		kvps := make([]ObjectProperty, node.Len())
		for i := range kvps {
			kvp := node.Index(i)

			k, kdiags := ParseExpr(kvp.Key)
			diags.Extend(kdiags...)

			v, vdiags := ParseExpr(kvp.Value)
			diags.Extend(vdiags...)

			assetOrArchive, adiags, ok := tryParseAssetOrArchive(k, v)
			diags.Extend(adiags...)
			if ok {
				if node.Len() != 1 {
					diags.Extend(syntax.NodeError(node, k.(*StringExpr).Value+" must have it's own object", ""))
					return nil, diags
				}
				// Not really an object, so we return early
				return assetOrArchive, diags
			}
			kvps[i] = ObjectProperty{syntax: kvp, Key: k, Value: v}
		}
		return ObjectSyntax(node, kvps...), diags
	default:
		return nil, syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("unexpected syntax node of type %T", node), "")}
	}
}

func getAssetOrArchive(name *StringExpr) (func(node syntax.Node, v Expr) Expr, syntax.Diagnostics, bool) {
	diag := func(expected string) syntax.Diagnostics {
		err := syntax.UnexpectedCasing(name.Syntax().Syntax().Range(), expected, name.GetValue())
		if err != nil {
			return syntax.Diagnostics{err}
		}
		return nil
	}
	switch strings.ToLower(name.GetValue()) {
	case "fn::stringasset":
		return func(node syntax.Node, v Expr) Expr { return StringAssetSyntax(node, name, v) }, diag("fn::stringAsset"), true
	case "fn::fileasset":
		return func(node syntax.Node, v Expr) Expr { return FileAssetSyntax(node, name, v) }, diag("fn::fileAsset"), true
	case "fn::remoteasset":
		return func(node syntax.Node, v Expr) Expr { return RemoteAssetSyntax(node, name, v) }, diag("fn::remoteAsset"), true
	case "fn::filearchive":
		return func(node syntax.Node, v Expr) Expr { return FileArchiveSyntax(node, name, v) }, diag("fn::fileArchive"), true
	case "fn::remotearchive":
		return func(node syntax.Node, v Expr) Expr { return RemoteArchiveSyntax(node, name, v) }, diag("fn::remoteArchive"), true
	default:
		return nil, nil, false
	}
}

// Attempts to parse an asset or archive. These are not normal `fn::*` objects
// because they are parsed as part of an `ObjectProperty` instead of an object.
// Note: because of the difference in parsing, this function does not identify
// assetArchive.
func tryParseAssetOrArchive(k, v Expr) (Expr, syntax.Diagnostics, bool) {
	fnName, ok := k.(*StringExpr)
	if !ok {
		return nil, nil, false
	}

	if fn, diags, ok := getAssetOrArchive(fnName); ok {
		return fn(k.Syntax(), v), diags, true
	}

	// Not a asset or archive
	return nil, nil, false
}

// BuiltinExpr represents a call to a builtin function.
type BuiltinExpr interface {
	Expr

	Name() *StringExpr
	Args() Expr

	isBuiltin()
}

type builtinNode struct {
	exprNode

	name *StringExpr
	args Expr
}

func builtin(node *syntax.ObjectNode, name *StringExpr, args Expr) builtinNode {
	return builtinNode{exprNode: expr(node), name: name, args: args}
}

func (*builtinNode) isBuiltin() {}

func (n *builtinNode) Name() *StringExpr {
	return n.name
}

func (n *builtinNode) Args() Expr {
	return n.args
}

// InvokeExpr is a function expression that invokes a Pulumi function by type token.
type InvokeExpr struct {
	builtinNode

	Token    *StringExpr
	CallArgs *ObjectExpr
	CallOpts InvokeOptionsDecl
	Return   *StringExpr
}

func InvokeSyntax(node *syntax.ObjectNode, name *StringExpr, args *ObjectExpr, token *StringExpr, callArgs *ObjectExpr, callOpts InvokeOptionsDecl, ret *StringExpr) *InvokeExpr {
	return &InvokeExpr{
		builtinNode: builtin(node, name, args),
		Token:       token,
		CallArgs:    callArgs,
		CallOpts:    callOpts,
		Return:      ret,
	}
}

func Invoke(token string, callArgs *ObjectExpr, callOpts InvokeOptionsDecl, ret string) *InvokeExpr {
	name, tok, retX := String("fn::invoke"), String(token), String(ret)

	entries := []ObjectProperty{{Key: String("function"), Value: tok}}
	if callArgs != nil {
		entries = append(entries, ObjectProperty{Key: String("arguments"), Value: callArgs})
	}

	entries = append(entries, ObjectProperty{Key: String("return"), Value: retX})

	return &InvokeExpr{
		builtinNode: builtin(nil, name, Object(entries...)),
		Token:       tok,
		CallArgs:    callArgs,
		CallOpts:    callOpts,
		Return:      retX,
	}
}

// ToJSON returns the underlying structure as a json string.
type ToJSONExpr struct {
	builtinNode

	Value Expr
}

func ToJSONSyntax(node *syntax.ObjectNode, name *StringExpr, args Expr) *ToJSONExpr {
	return &ToJSONExpr{
		builtinNode: builtin(node, name, args),
		Value:       args,
	}
}

func ToJSON(value Expr) *ToJSONExpr {
	name := String("fn::toJSON")
	return ToJSONSyntax(nil, name, value)
}

// JoinExpr appends a set of values into a single value, separated by the specified delimiter.
// If a delimiter is the empty string, the set of values are concatenated with no delimiter.
type JoinExpr struct {
	builtinNode

	Delimiter Expr
	Values    Expr
}

func JoinSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, delimiter Expr, values Expr) *JoinExpr {
	return &JoinExpr{
		builtinNode: builtin(node, name, args),
		Delimiter:   delimiter,
		Values:      values,
	}
}

func Join(delimiter Expr, values *ListExpr) *JoinExpr {
	name := String("fn::join")
	return &JoinExpr{
		builtinNode: builtin(nil, name, List(delimiter, values)),
		Delimiter:   delimiter,
		Values:      values,
	}
}

// Splits a string into a list by a delimiter
type SplitExpr struct {
	builtinNode

	Delimiter Expr
	Source    Expr
}

func SplitSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr) *SplitExpr {
	elems := args.Elements
	contract.Assertf(len(elems) == 2, "Must have exactly 2 elements")
	return &SplitExpr{
		builtinNode: builtin(node, name, args),
		Delimiter:   elems[0],
		Source:      elems[1],
	}
}

func Split(delimiter, source Expr) *SplitExpr {
	name := String("fn::split")
	return &SplitExpr{
		builtinNode: builtin(nil, name, List(delimiter, source)),
		Delimiter:   delimiter,
		Source:      source,
	}
}

// SelectExpr returns a single object from a list of objects by index.
type SelectExpr struct {
	builtinNode

	Index  Expr
	Values Expr
}

func SelectSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, index Expr, values Expr) *SelectExpr {
	return &SelectExpr{
		builtinNode: builtin(node, name, args),
		Index:       index,
		Values:      values,
	}
}

func Select(index Expr, values Expr) *SelectExpr {
	name := String("fn::select")
	return &SelectExpr{
		builtinNode: builtin(nil, name, List(index, values)),
		Index:       index,
		Values:      values,
	}
}

type ToBase64Expr struct {
	builtinNode

	Value Expr
}

func ToBase64Syntax(node *syntax.ObjectNode, name *StringExpr, args Expr) *ToBase64Expr {
	return &ToBase64Expr{
		builtinNode: builtin(node, name, args),
		Value:       args,
	}
}

type FromBase64Expr struct {
	builtinNode

	Value Expr
}

func FromBase64Syntax(node *syntax.ObjectNode, name *StringExpr, args Expr) *FromBase64Expr {
	return &FromBase64Expr{
		builtinNode: builtin(node, name, args),
		Value:       args,
	}
}

type AssetOrArchiveExpr interface {
	Expr
	isAssetOrArchive()
}

type StringAssetExpr struct {
	builtinNode
	Source Expr
}

func (*StringAssetExpr) isAssetOrArchive() {}

func StringAssetSyntax(node syntax.Node, name *StringExpr, source Expr) *StringAssetExpr {
	return &StringAssetExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type FileAssetExpr struct {
	builtinNode
	Source Expr
}

func (*FileAssetExpr) isAssetOrArchive() {}

func FileAssetSyntax(node syntax.Node, name *StringExpr, source Expr) *FileAssetExpr {
	return &FileAssetExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type RemoteAssetExpr struct {
	builtinNode
	Source Expr
}

func (*RemoteAssetExpr) isAssetOrArchive() {}

func RemoteAssetSyntax(node syntax.Node, name *StringExpr, source Expr) *RemoteAssetExpr {
	return &RemoteAssetExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type FileArchiveExpr struct {
	builtinNode
	Source Expr
}

func (*FileArchiveExpr) isAssetOrArchive() {}

func FileArchiveSyntax(node syntax.Node, name *StringExpr, source Expr) *FileArchiveExpr {
	return &FileArchiveExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type RemoteArchiveExpr struct {
	builtinNode
	Source Expr
}

func (*RemoteArchiveExpr) isAssetOrArchive() {}

func RemoteArchiveSyntax(node syntax.Node, name *StringExpr, source Expr) *RemoteArchiveExpr {
	return &RemoteArchiveExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type AssetArchiveExpr struct {
	builtinNode
	AssetOrArchives map[string]Expr
}

func (*AssetArchiveExpr) isAssetOrArchive() {}

func AssetArchiveSyntax(node *syntax.ObjectNode, name *StringExpr, args *ObjectExpr, assetsOrArchives map[string]Expr) *AssetArchiveExpr {
	return &AssetArchiveExpr{
		builtinNode:     builtin(node, name, args),
		AssetOrArchives: assetsOrArchives,
	}
}

// StackReferenceExpr gets an output of another stack for use in this deployment.
type StackReferenceExpr struct {
	builtinNode

	StackName    *StringExpr
	PropertyName Expr
}

func StackReferenceSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, stackName *StringExpr, propertyName Expr) *StackReferenceExpr {
	return &StackReferenceExpr{
		builtinNode:  builtin(node, name, args),
		StackName:    stackName,
		PropertyName: propertyName,
	}
}

func StackReference(stackName string, propertyName Expr) *StackReferenceExpr {
	name, stackNameX := String("fn::stackReference"), String(stackName)

	return &StackReferenceExpr{
		builtinNode:  builtin(nil, name, List(stackNameX, propertyName)),
		StackName:    stackNameX,
		PropertyName: propertyName,
	}
}

type SecretExpr struct {
	builtinNode

	Value Expr
}

func SecretSyntax(node *syntax.ObjectNode, name *StringExpr, args Expr) *SecretExpr {
	return &SecretExpr{
		builtinNode: builtin(node, name, args),
		Value:       args,
	}
}

type ReadFileExpr struct {
	builtinNode
	Path Expr
}

func ReadFileSyntax(node syntax.Node, name *StringExpr, path Expr) *ReadFileExpr {
	return &ReadFileExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: path},
		Path:        path,
	}
}

type RFC3339ToUnixExpr struct {
	builtinNode

	Value Expr
}

func RFC3339ToUnixSyntax(node *syntax.ObjectNode, name *StringExpr, args Expr) *RFC3339ToUnixExpr {
	return &RFC3339ToUnixExpr{
		builtinNode: builtin(node, name, args),
		Value:       args,
	}
}

func parseReadFile(node *syntax.ObjectNode, name *StringExpr, path Expr) (Expr, syntax.Diagnostics) {
	return ReadFileSyntax(node, name, path), nil
}

func tryParseFunction(node *syntax.ObjectNode) (Expr, syntax.Diagnostics, bool) {
	if node.Len() != 1 {
		return nil, nil, false
	}

	kvp := node.Index(0)

	if _, _, ok := getAssetOrArchive(StringSyntax(kvp.Key)); ok {
		// We will parse this node as an asset or archive later, so we don't need to do it now
		return nil, nil, false
	}

	var parse func(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics)
	var diags syntax.Diagnostics
	set := func(expected string, parseFn func(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics)) {
		diags.Extend(syntax.UnexpectedCasing(kvp.Key.Syntax().Range(), expected, kvp.Key.Value()))
		parse = parseFn
	}
	switch strings.ToLower(kvp.Key.Value()) {
	case "fn::invoke":
		set("fn::invoke", parseInvoke)
	case "fn::join":
		set("fn::join", parseJoin)
	case "fn::tojson":
		set("fn::toJSON", parseToJSON)
	case "fn::tobase64":
		set("fn::toBase64", parseToBase64)
	case "fn::frombase64":
		set("fn::fromBase64", parseFromBase64)
	case "fn::select":
		set("fn::select", parseSelect)
	case "fn::split":
		set("fn::split", parseSplit)
	case "fn::stackreference":
		set("fn::stackReference", parseStackReference)
		diags = append(diags, syntax.Warning(kvp.Key.Syntax().Range(),
			`'fn::stackReference' is deprecated; please use 'pulumi:pulumi:StackReference' instead`,
			`see "https://www.pulumi.com/docs/intro/concepts/stack/#stackreferences for more info.`))
	case "fn::assetarchive":
		set("fn::assetArchive", parseAssetArchive)
	case "fn::secret":
		set("fn::secret", parseSecret)
	case "fn::readfile":
		set("fn::readFile", parseReadFile)
	case "fn::rfc3339ToUnix":
		set("fn::rfc3339ToUnix", parseRFC3339ToUnix)
	default:
		k := kvp.Key.Value()
		// fn::invoke can be called as fn::${pkg}:${module}(:${name})?
		// error is thrown if regex pattern cannot be parsed — handled by `regex.MustCompile(fnInvokeRegex)`
		if fnInvokeRegex.MatchString(strings.ToLower(k)) {
			// transform the node into standard fn::invoke format
			fnVal := k[4:]
			if _, ok := kvp.Value.(*syntax.ObjectNode); ok {
				kvp.Value = syntax.Object(
					syntax.ObjectPropertyDef{
						Key:   syntax.StringSyntax(kvp.Syntax, "arguments"),
						Value: kvp.Value,
					},
					syntax.ObjectPropertyDef{
						Key:   syntax.StringSyntax(kvp.Syntax, "function"),
						Value: syntax.String(fnVal),
					},
				)
			} else {
				kvp.Value = syntax.Object(
					syntax.ObjectPropertyDef{
						Key:   syntax.StringSyntax(kvp.Syntax, "function"),
						Value: syntax.String(fnVal),
					},
				)
			}
			parse = parseInvoke
			break
		} else if strings.HasPrefix(strings.ToLower(k), "fn::") {
			diags = append(diags, syntax.Warning(kvp.Key.Syntax().Range(),
				"'fn::' is a reserved prefix",
				fmt.Sprintf("If you need to use the raw key '%s',"+
					" please open an issue at https://github.com/pulumi/pulumi-yaml/issues", k)))
		}
		return nil, diags, false
	}

	name := StringSyntax(kvp.Key)

	args, adiags := ParseExpr(kvp.Value)
	diags.Extend(adiags...)

	expr, xdiags := parse(node, name, args)
	diags.Extend(xdiags...)

	if expr == nil {
		expr = ObjectSyntax(node, ObjectProperty{
			syntax: kvp,
			Key:    name,
			Value:  args,
		})
	}

	return expr, diags, true
}

func parseInvoke(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	obj, ok := args.(*ObjectExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to fn::invoke must be an object containing 'function', 'arguments', 'options', and 'return'", "")}
	}

	var functionExpr, argumentsExpr, returnExpr Expr
	var diags syntax.Diagnostics
	opts := InvokeOptionsDecl{}

	for i := 0; i < len(obj.Entries); i++ {
		kvp := obj.Entries[i]
		if str, ok := kvp.Key.(*StringExpr); ok {
			switch strings.ToLower(str.Value) {
			case "function":
				diags.Extend(syntax.UnexpectedCasing(str.syntax.Syntax().Range(), "function", str.GetValue()))
				functionExpr = kvp.Value
			case "arguments":
				diags.Extend(syntax.UnexpectedCasing(str.syntax.Syntax().Range(), "arguments", str.GetValue()))
				argumentsExpr = kvp.Value
			case "options":
				diags.Extend(syntax.UnexpectedCasing(str.syntax.Syntax().Range(), "options", str.GetValue()))
				diags.Extend(parseRecord("invokeOptions", &opts, kvp.syntax.Value, true)...)
				if diags.HasErrors() {
					return nil, diags
				}
			case "return":
				diags.Extend(syntax.UnexpectedCasing(str.syntax.Syntax().Range(), "return", str.GetValue()))
				returnExpr = kvp.Value
			}
		}
	}

	function, ok := functionExpr.(*StringExpr)
	if !ok {
		if functionExpr == nil {
			diags.Extend(ExprError(obj, "missing function name ('function')", ""))
		} else {
			diags.Extend(ExprError(functionExpr, "function name must be a string literal", ""))
		}
	}

	arguments, ok := argumentsExpr.(*ObjectExpr)
	if !ok && argumentsExpr != nil {
		diags.Extend(ExprError(argumentsExpr, "function arguments ('arguments') must be an object", ""))
	}

	ret, ok := returnExpr.(*StringExpr)
	if !ok && returnExpr != nil {
		diags.Extend(ExprError(returnExpr, "return directive must be a string literal", ""))
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return InvokeSyntax(node, name, obj, function, arguments, opts, ret), diags
}

func parseJoin(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to fn::join must be a two-valued list", "")}
	}

	return JoinSyntax(node, name, list, list.Elements[0], list.Elements[1]), nil
}

func parseToJSON(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return ToJSONSyntax(node, name, args), nil
}

func parseSelect(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to fn::select must be a two-valued list", "")}
	}

	index := list.Elements[0]
	values := list.Elements[1]
	return SelectSyntax(node, name, list, index, values), nil
}

func parseSplit(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "The argument to fn::split must be a two-values list", "")}
	}

	return SplitSyntax(node, name, list), nil
}

func parseToBase64(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return ToBase64Syntax(node, name, args), nil
}

func parseFromBase64(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return FromBase64Syntax(node, name, args), nil
}

func parseStackReference(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to fn::stackReference must be a two-valued list", "")}
	}

	stackName, ok := list.Elements[0].(*StringExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(args, "the first argument to fn::stackReference must be a string literal", "")}
	}

	return StackReferenceSyntax(node, name, list, stackName, list.Elements[1]), nil
}

func parseSecret(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return SecretSyntax(node, name, args), nil
}

// We expect the following format
//
//	fn::assetArchive:
//	  path:
//	    AssetOrArchive
//
// Where `AssetOrArchive` is an object.
func parseAssetArchive(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	const mustObjectMsg string = "the argument to fn::assetArchive must be an object"
	const mustStringMsg string = "keys in fn::assetArchive arguments must be string literals"
	obj, ok := args.(*ObjectExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(args, mustObjectMsg, "")}
	}
	var diags syntax.Diagnostics
	assetOrArchives := map[string]Expr{}
	for _, kv := range obj.Entries {
		var tdiags syntax.Diagnostics // Diags local to this iteration
		k, ok := kv.Key.(*StringExpr) // the path for this entry
		if !ok {
			tdiags.Extend(ExprError(kv.Key, mustStringMsg, ""))
		}
		v, ok := kv.Value.(AssetOrArchiveExpr)
		if !ok {
			tdiags.Extend(ExprError(kv.Value, fmt.Sprintf("value must be an asset or an archive, not a %T", kv.Value), ""))
		}
		if !tdiags.HasErrors() {
			assetOrArchives[k.Value] = v
		}
		diags.Extend(tdiags...)
	}
	return AssetArchiveSyntax(node, name, obj, assetOrArchives), diags
}

func parseRFC3339ToUnix(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return RFC3339ToUnixSyntax(node, name, args), nil
}
