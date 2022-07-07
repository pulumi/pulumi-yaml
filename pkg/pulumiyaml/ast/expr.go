// Copyright 2022, Pulumi Corporation.  All rights reserved.

package ast

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

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
// - *syntax.{Null,Boolean,Number}Node is parsed as a *{Null,Boolean,Number}Expr.
// - *syntax.ListNode is parsed as a *ListExpr.
// - *syntax.StringNode is parsed as an *InterpolateExpr, a *SymbolExpr, or a *StringExpr. The node's literal is first
//   parsed as an interpolated string. If the result contains a single property access with no surrounding text, (i.e.
//   the string is of the form "${resource.property}", it is treated as a symbol. If the result contains no property
//   accesses, it is treated as a string literal. Otherwise, it it treated as an interpolated string.
// - *syntax.ObjectNode is parses as either an *ObjectExpr or a BuiltinExpr. If the object contains a single key and
//   that key names a builtin function ("Fn::Invoke", "Fn::Join", "Fn::Select",
//   "Fn::*Asset", "Fn::*Archive", or "Fn::StackReference"), then the object is parsed as the corresponding BuiltinExpr.
//   Otherwise, the object is parsed as a *syntax.ObjectNode.
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
					return StringSyntax(node), diags
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
		if x, diags, ok := tryParseFunction(node); ok {
			return x, diags
		}

		var diags syntax.Diagnostics

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

// Attempts to parse an asset or archive. These are not normal `Fn::*` objects
// because they are parsed as part of an `ObjectProperty` instead of an object.
// Note: because of the difference in parsing, this function does not identify
// AssetArchive.
func tryParseAssetOrArchive(k, v Expr) (Expr, syntax.Diagnostics, bool) {
	diags := syntax.Diagnostics{}
	checkStringExpr := func(kind string, v Expr) (*StringExpr, bool) {
		if s, ok := v.(*StringExpr); ok {
			return s, true
		}
		diags.Extend(syntax.NodeError(v.Syntax(), fmt.Sprintf("The argument to %s must be a string literal", kind), ""))
		return nil, false
	}
	fnName, ok := k.(*StringExpr)
	if !ok {
		return nil, nil, false
	}
	switch fnName.Value {
	case "Fn::StringAsset":
		s, ok := checkStringExpr(fnName.Value, v)
		if !ok {
			return nil, diags, true
		}
		return StringAssetSyntax(k.Syntax(), fnName, s), diags, true
	case "Fn::FileAsset":
		s, ok := checkStringExpr(fnName.Value, v)
		if !ok {
			return nil, diags, true
		}
		return FileAssetSyntax(k.Syntax(), fnName, s), diags, true
	case "Fn::RemoteAsset":
		s, ok := checkStringExpr(fnName.Value, v)
		if !ok {
			return nil, diags, true
		}
		return RemoteAssetSyntax(k.Syntax(), fnName, s), diags, true
	case "Fn::FileArchive":
		s, ok := checkStringExpr(fnName.Value, v)
		if !ok {
			return nil, diags, true
		}
		return FileArchiveSyntax(k.Syntax(), fnName, s), diags, true
	case "Fn::RemoteArchive":
		s, ok := checkStringExpr(fnName.Value, v)
		if !ok {
			return nil, diags, true
		}
		return RemoteArchiveSyntax(k.Syntax(), fnName, s), diags, true
	default:
		// Not a asset or archive
		return nil, nil, false
	}
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
	Return   *StringExpr
}

func InvokeSyntax(node *syntax.ObjectNode, name *StringExpr, args *ObjectExpr, token *StringExpr, callArgs *ObjectExpr, ret *StringExpr) *InvokeExpr {
	return &InvokeExpr{
		builtinNode: builtin(node, name, args),
		Token:       token,
		CallArgs:    callArgs,
		Return:      ret,
	}
}

func Invoke(token string, callArgs *ObjectExpr, ret string) *InvokeExpr {
	name, tok, retX := String("Fn::Invoke"), String(token), String(ret)

	entries := []ObjectProperty{{Key: String("Function"), Value: tok}}
	if callArgs != nil {
		entries = append(entries, ObjectProperty{Key: String("Arguments"), Value: callArgs})
	}
	entries = append(entries, ObjectProperty{Key: String("Return"), Value: retX})

	return &InvokeExpr{
		builtinNode: builtin(nil, name, Object(entries...)),
		Token:       tok,
		CallArgs:    callArgs,
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
	name := String("Fn::ToJSON")
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
	name := String("Fn::Join")
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
	name := String("Fn::Split")
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
	name := String("Fn::Select")
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
	Source *StringExpr
}

func (*StringAssetExpr) isAssetOrArchive() {}

func StringAssetSyntax(node syntax.Node, name, source *StringExpr) *StringAssetExpr {
	return &StringAssetExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type FileAssetExpr struct {
	builtinNode
	Source *StringExpr
}

func (*FileAssetExpr) isAssetOrArchive() {}

func FileAssetSyntax(node syntax.Node, name, source *StringExpr) *FileAssetExpr {
	return &FileAssetExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type RemoteAssetExpr struct {
	builtinNode
	Source *StringExpr
}

func (*RemoteAssetExpr) isAssetOrArchive() {}

func RemoteAssetSyntax(node syntax.Node, name, source *StringExpr) *RemoteAssetExpr {
	return &RemoteAssetExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type FileArchiveExpr struct {
	builtinNode
	Source *StringExpr
}

func (*FileArchiveExpr) isAssetOrArchive() {}

func FileArchiveSyntax(node syntax.Node, name, source *StringExpr) *FileArchiveExpr {
	return &FileArchiveExpr{
		builtinNode: builtinNode{exprNode: expr(node), name: name, args: source},
		Source:      source,
	}
}

type RemoteArchiveExpr struct {
	builtinNode
	Source *StringExpr
}

func (*RemoteArchiveExpr) isAssetOrArchive() {}

func RemoteArchiveSyntax(node syntax.Node, name, source *StringExpr) *RemoteArchiveExpr {
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
	name, stackNameX := String("Fn::StackReference"), String(stackName)

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

func parseReadFile(node *syntax.ObjectNode, name *StringExpr, path Expr) (Expr, syntax.Diagnostics) {
	return ReadFileSyntax(node, name, path), nil
}

func tryParseFunction(node *syntax.ObjectNode) (Expr, syntax.Diagnostics, bool) {
	if node.Len() != 1 {
		return nil, nil, false
	}

	kvp := node.Index(0)

	var parse func(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics)
	switch kvp.Key.Value() {
	case "Fn::Invoke":
		parse = parseInvoke
	case "Fn::Join":
		parse = parseJoin
	case "Fn::ToJSON":
		parse = parseToJSON
	case "Fn::ToBase64":
		parse = parseToBase64
	case "Fn::FromBase64":
		parse = parseFromBase64
	case "Fn::Select":
		parse = parseSelect
	case "Fn::Split":
		parse = parseSplit
	case "Fn::StackReference":
		parse = parseStackReference
	case "Fn::AssetArchive":
		parse = parseAssetArchive
	case "Fn::Secret":
		parse = parseSecret
	case "Fn::ReadFile":
		parse = parseReadFile
	default:
		return nil, nil, false
	}

	var diags syntax.Diagnostics

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
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Invoke must be an object containing 'Function', 'Arguments', and 'Return'", "")}
	}

	var functionExpr, argumentsExpr, returnExpr Expr
	for i := 0; i < len(obj.Entries); i++ {
		kvp := obj.Entries[i]
		if str, ok := kvp.Key.(*StringExpr); ok {
			switch str.Value {
			case "Function":
				functionExpr = kvp.Value
			case "Arguments":
				argumentsExpr = kvp.Value
			case "Return":
				returnExpr = kvp.Value
			}
		}
	}

	var diags syntax.Diagnostics

	function, ok := functionExpr.(*StringExpr)
	if !ok {
		if functionExpr == nil {
			diags.Extend(ExprError(obj, "missing function name ('Function')", ""))
		} else {
			diags.Extend(ExprError(functionExpr, "function name must be a string literal", ""))
		}
	}

	arguments, ok := argumentsExpr.(*ObjectExpr)
	if !ok && argumentsExpr != nil {
		diags.Extend(ExprError(argumentsExpr, "function arguments ('Arguments') must be an object", ""))
	}

	ret, ok := returnExpr.(*StringExpr)
	if !ok && returnExpr != nil {
		diags.Extend(ExprError(returnExpr, "return directive must be a string literal", ""))
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return InvokeSyntax(node, name, obj, function, arguments, ret), diags
}

func parseJoin(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Join must be a two-valued list", "")}
	}

	return JoinSyntax(node, name, list, list.Elements[0], list.Elements[1]), nil
}

func parseToJSON(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return ToJSONSyntax(node, name, args), nil
}

func parseSelect(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Select must be a two-valued list", "")}
	}

	index := list.Elements[0]
	values := list.Elements[1]
	return SelectSyntax(node, name, list, index, values), nil
}

func parseSplit(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "The argument to Fn::Select must be a two-values list", "")}
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
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::StackReference must be a two-valued list", "")}
	}

	stackName, ok := list.Elements[0].(*StringExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(args, "the first argument to Fn::StackReference must be a string literal", "")}
	}

	return StackReferenceSyntax(node, name, list, stackName, list.Elements[1]), nil
}

func parseSecret(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return SecretSyntax(node, name, args), nil
}

// We expect the following format
// Fn::AssetArchive:
//   path:
//     AssetOrArchive
//
// Where `AssetOrArchive` is an object.
func parseAssetArchive(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	const mustObjectMsg string = "the argument to Fn::AssetArchive must be an object"
	const mustStringMsg string = "keys in Fn::AssetArchive arguments must be string literals"
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
