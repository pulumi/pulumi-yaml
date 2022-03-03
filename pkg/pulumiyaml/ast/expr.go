// Copyright 2022, Pulumi Corporation.  All rights reserved.

package ast

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
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

// GetValue returns the expression's value. If the receiever is null, GetValue returns the empty string.
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
	if len(diags) != 0 {
		return nil, diags
	}

	return &InterpolateExpr{
		exprNode: expr(node),
		Parts:    parts,
	}, nil
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
	return ListSyntax(nil, elements...)
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
	return ObjectSyntax(nil, entries...)
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
//   that key names a builtin function ("Ref", "Fn::GetAtt", "Fn::Invoke", "Fn::Join", "Fn::Sub", "Fn::Select",
//   "Fn::Asset", or "Fn::StackReference"), then the object is parsed as the corresponding BuiltinExpr. Otherwise, the
//   object is parsed as a *syntax.ObjectNode.
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

			kvps[i] = ObjectProperty{syntax: kvp, Key: k, Value: v}
		}
		return ObjectSyntax(node, kvps...), diags
	default:
		return nil, syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("unexpected syntax node of type %T", node), "")}
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

// RefExpr is a function expression that computes a reference to another resource.
type RefExpr struct {
	builtinNode

	ResourceName *StringExpr
}

func RefSyntax(node *syntax.ObjectNode, name *StringExpr, resourceName *StringExpr) *RefExpr {
	return &RefExpr{
		builtinNode:  builtin(node, name, resourceName),
		ResourceName: resourceName,
	}
}

func Ref(resourceName string) *RefExpr {
	return &RefExpr{ResourceName: String(resourceName)}
}

// GetAttExpr is a function expression that accesses an output property of another resources.
type GetAttExpr struct {
	builtinNode

	ResourceName *StringExpr
	// TODO: CloudFormation allows nested Ref in PropertyName, so this could be an Expr
	PropertyName *StringExpr
}

func GetAttSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, resourceName, propertyName *StringExpr) *GetAttExpr {
	return &GetAttExpr{
		builtinNode:  builtin(node, name, args),
		ResourceName: resourceName,
		PropertyName: propertyName,
	}
}

func GetAtt(resourceName, propertyName string) *GetAttExpr {
	name, resName, propName := String("Fn::GetAtt"), String(resourceName), String(propertyName)
	return &GetAttExpr{
		builtinNode:  builtin(nil, name, List(resName, propName)),
		ResourceName: resName,
		PropertyName: propName,
	}
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
	// TODO: CloudFormation allows nested functions to produce the Values - so this should be an Expr
	Values *ListExpr
}

func JoinSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, delimiter Expr, values *ListExpr) *JoinExpr {
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

func SplitSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, delimiter, source Expr) *SplitExpr {
	return &SplitExpr{
		builtinNode: builtin(node, name, args),
		Delimiter:   delimiter,
		Source:      source,
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

	Index Expr
	// TODO: CloudFormation allows nested functions to produce the Values - so this should be an Expr
	Values *ListExpr
}

func SelectSyntax(node *syntax.ObjectNode, name *StringExpr, args *ListExpr, index Expr, values *ListExpr) *SelectExpr {
	return &SelectExpr{
		builtinNode: builtin(node, name, args),
		Index:       index,
		Values:      values,
	}
}

func Select(index Expr, values *ListExpr) *SelectExpr {
	name := String("Fn::Select")
	return &SelectExpr{
		builtinNode: builtin(nil, name, List(index, values)),
		Index:       index,
		Values:      values,
	}
}

// SubExpr substitutes variables in an input string with values that you specify. In your templates, you can use this
// function to construct commands or outputs that include values that aren't available until you create or update a
// stack.
type SubExpr struct {
	builtinNode

	Interpolate   *InterpolateExpr
	Substitutions *ObjectExpr
}

func SubSyntax(node *syntax.ObjectNode, name *StringExpr, args Expr, interpolate *InterpolateExpr, substitutions *ObjectExpr) *SubExpr {
	return &SubExpr{
		builtinNode:   builtin(node, name, args),
		Interpolate:   interpolate,
		Substitutions: substitutions,
	}
}

func Sub(interpolate *InterpolateExpr, substitutions *ObjectExpr) *SubExpr {
	name := String("Fn::Sub")

	args := Expr(interpolate)
	if substitutions != nil {
		args = List(interpolate, substitutions)
	}

	return &SubExpr{
		builtinNode:   builtin(nil, name, args),
		Interpolate:   interpolate,
		Substitutions: substitutions,
	}
}

type ToBase64Expr struct {
	builtinNode

	Value Expr
}

func ToBase64Syntax(node *syntax.ObjectNode, name *StringExpr, args Expr, value Expr) *ToBase64Expr {
	return &ToBase64Expr{
		builtinNode: builtin(node, name, args),
		Value:       value,
	}
}

// AssetExpr references a file either on disk ("File"), created in memory ("String") or accessed remotely ("Remote").
type AssetExpr struct {
	builtinNode

	Kind *StringExpr
	Path *StringExpr
}

func AssetSyntax(node *syntax.ObjectNode, name *StringExpr, args *ObjectExpr, kind *StringExpr, path *StringExpr) *AssetExpr {
	return &AssetExpr{
		builtinNode: builtin(node, name, args),
		Kind:        kind,
		Path:        path,
	}
}

func Asset(kind string, path string) *AssetExpr {
	name, kindX, pathX := String("Fn::Asset"), String(kind), String(path)

	return &AssetExpr{
		builtinNode: builtin(nil, name, Object(ObjectProperty{Key: kindX, Value: pathX})),
		Kind:        kindX,
		Path:        pathX,
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

func tryParseFunction(node *syntax.ObjectNode) (Expr, syntax.Diagnostics, bool) {
	if node.Len() != 1 {
		return nil, nil, false
	}

	kvp := node.Index(0)

	var parse func(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics)
	switch kvp.Key.Value() {
	case "Ref":
		parse = parseRef
	case "Fn::GetAtt":
		parse = parseGetAtt
	case "Fn::Invoke":
		parse = parseInvoke
	case "Fn::Join":
		parse = parseJoin
	case "Fn::ToJSON":
		parse = parseToJSON
	case "Fn::Sub":
		parse = parseSub
	case "Fn::ToBase64":
		parse = parseToBase64
	case "Fn::Select":
		parse = parseSelect
	case "Fn::Asset":
		parse = parseAsset
	case "Fn::StackReference":
		parse = parseStackReference
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

// parseRef reads and validates the arguments to Ref.
func parseRef(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	str, ok := args.(*StringExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(args, "the resource name for Ref must be a string literal", "")}
	}
	return RefSyntax(node, name, str), nil
}

// parseGetAtt reads and validates the arguments to Fn::GetAtt.
func parseGetAtt(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::GetAtt must be a two-valued list", "")}
	}

	var diags syntax.Diagnostics

	resourceName, ok := list.Elements[0].(*StringExpr)
	if !ok {
		diags.Extend(ExprError(list.Elements[0], "the first argument to Fn::GetAtt must be a string literal", ""))
	}

	propertyName, ok := list.Elements[1].(*StringExpr)
	if !ok {
		diags.Extend(ExprError(list.Elements[0], "the second argument to Fn::GetAtt must be a string literal", ""))
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return GetAttSyntax(node, name, list, resourceName, propertyName), diags
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

	values, ok := list.Elements[1].(*ListExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(list.Elements[1], "the second argument to Fn::Join must be a list", "")}
	}

	return JoinSyntax(node, name, list, list.Elements[0], values), nil
}

func parseToJSON(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	return ToJSONSyntax(node, name, args), nil
}

func parseSelect(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	list, ok := args.(*ListExpr)
	if !ok || len(list.Elements) != 2 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Select must be a two-valued list", "")}
	}

	values, ok := list.Elements[1].(*ListExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(list.Elements[1], "the second argument to Fn::Select must be a list", "")}
	}

	return SelectSyntax(node, name, list, list.Elements[0], values), nil
}

func parseSub(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	var diags syntax.Diagnostics

	// Read and validate the arguments to Fn::Sub.
	var template Expr
	var substitutions *ObjectExpr
	switch args := args.(type) {
	case *ListExpr:
		if len(args.Elements) != 2 {
			return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Sub must be a two-valued list or a string", "")}
		}

		subs, ok := args.Elements[1].(*ObjectExpr)
		if !ok {
			diags.Extend(ExprError(args.Elements[1], "the second argument to Fn::Sub must be an object", ""))
		} else {
			for _, kvp := range subs.Entries {
				if _, ok := kvp.Key.(*StringExpr); !ok {
					diags.Extend(ExprError(kvp.Key, "substitution name must be a string literal", ""))
				}
			}
		}
		substitutions = subs

		template = args.Elements[0]

	case *InterpolateExpr, *SymbolExpr, *StringExpr:
		template = args
	default:
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Sub must be a two-valued list or a string", "")}
	}

	var interpolate *InterpolateExpr
	switch template := template.(type) {
	case *InterpolateExpr:
		interpolate = template
	case *SymbolExpr:
		interpolate = &InterpolateExpr{
			exprNode: template.exprNode,
			Parts:    []Interpolation{{Value: template.Property}},
		}
	case *StringExpr:
		interpolate = &InterpolateExpr{
			exprNode: template.exprNode,
			Parts:    []Interpolation{{Text: template.Value}},
		}
	default:
		diags.Extend(ExprError(template, "the sfirst argument to Fn::Sub must be a string", ""))
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return SubSyntax(node, name, args, interpolate, substitutions), diags
}

func parseToBase64(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	str, ok := args.(*StringExpr)
	if !ok {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::ToBase64 must be a string", "")}
	}
	return ToBase64Syntax(node, name, args, str), nil
}

func parseAsset(node *syntax.ObjectNode, name *StringExpr, args Expr) (Expr, syntax.Diagnostics) {
	obj, ok := args.(*ObjectExpr)
	if !ok || len(obj.Entries) != 1 {
		return nil, syntax.Diagnostics{ExprError(args, "the argument to Fn::Asset must be an object containing one of 'File', 'String', or 'Remote'", "")}
	}

	var diags syntax.Diagnostics

	kvp := obj.Entries[0]
	kind, ok := kvp.Key.(*StringExpr)
	if !ok {
		diags.Extend(ExprError(kvp.Key, "the asset kind must be a string literal", ""))
	} else if kind.Value != "File" && kind.Value != "String" && kind.Value != "Remote" && kind.Value != "FileArchive" && kind.Value != "RemoteArchive" {
		diags.Extend(ExprError(kvp.Key, "the asset kind must be one of 'File', 'String', 'Remote', 'FileArchive', or 'RemoteArchive'", ""))
	}

	path, ok := kvp.Value.(*StringExpr)
	if !ok {
		diags.Extend(ExprError(kvp.Value, "the asset parameter must be a string literal", ""))
	}

	if diags.HasErrors() {
		return nil, diags
	}

	return AssetSyntax(node, name, obj, kind, path), diags
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
