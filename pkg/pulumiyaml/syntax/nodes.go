// Copyright 2022, Pulumi Corporation.  All rights reserved.

package syntax

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// A Node represents a single node in an object tree.
type Node interface {
	fmt.Stringer

	Syntax() Syntax

	isNode()
}

type node struct {
	syntax Syntax
}

func (n *node) Syntax() Syntax {
	if n == nil {
		return nil
	}
	return n.syntax
}

func (n *node) isNode() {
}

// A NullNode represents a null literal.
type NullNode struct {
	node
}

// NullSyntax creates a new null literal node with associated syntax.
func NullSyntax(syntax Syntax) *NullNode {
	return &NullNode{node: node{syntax: syntax}}
}

// Null creates a new null literal node.
func Null() *NullNode {
	return NullSyntax(NoSyntax)
}

func (*NullNode) String() string {
	return "null"
}

// A BooleanNode represents a boolean literal.
type BooleanNode struct {
	node

	value bool
}

// BooleanSyntax creates a new boolean literal node with the given value and associated syntax.
func BooleanSyntax(syntax Syntax, value bool) *BooleanNode {
	return &BooleanNode{node: node{syntax: syntax}, value: value}
}

// Boolean creates a new boolean literal node with the given value.
func Boolean(value bool) *BooleanNode {
	return BooleanSyntax(NoSyntax, value)
}

func (n *BooleanNode) String() string {
	if n.value {
		return "true"
	}
	return "false"
}

// Value returns the boolean literal's value.
func (n *BooleanNode) Value() bool {
	return n.value
}

// A NumberNode represents a number literal.
type NumberNode struct {
	node

	value float64
}

// NumberSyntax creates a new number literal node with the given value and associated syntax.
func NumberSyntax(syntax Syntax, value float64) *NumberNode {
	return &NumberNode{node: node{syntax: syntax}, value: value}
}

// Number creates a new number literal node with the given value.
func Number(value float64) *NumberNode {
	return NumberSyntax(NoSyntax, value)
}

// Value returns the number literal's value.
func (n *NumberNode) Value() float64 {
	return n.value
}

func (n *NumberNode) String() string {
	return strconv.FormatFloat(n.value, 'f', -1, 64)
}

// A StringNode represents a string literal.
type StringNode struct {
	node

	value string
}

// String creates a new string literal node with the given value and associated syntax.
func StringSyntax(syntax Syntax, value string) *StringNode {
	return &StringNode{
		node:  node{syntax: syntax},
		value: value,
	}
}

// String creates a new string literal node with the given value.
func String(value string) *StringNode {
	return StringSyntax(NoSyntax, value)
}

// String returns the string literal's value.
func (n *StringNode) String() string {
	return n.value
}

// Value returns the string literal's value.
func (n *StringNode) Value() string {
	return n.value
}

// A ListNode represents a list of nodes.
type ListNode struct {
	node

	elements []Node
}

// ListSyntax creates a new list node with the given elements and associated syntax.
func ListSyntax(syntax Syntax, elements ...Node) *ListNode {
	return &ListNode{node: node{syntax: syntax}, elements: elements}
}

// List creates a new list node with the given elements.
func List(elements ...Node) *ListNode {
	return ListSyntax(NoSyntax, elements...)
}

// Len returns the number of elements in the list.
func (n *ListNode) Len() int {
	return len(n.elements)
}

// Index returns the i'th element of the list.
func (n *ListNode) Index(i int) Node {
	return n.elements[i]
}

func (n *ListNode) String() string {
	if len(n.elements) == 0 {
		return "[ ]"
	}
	s := make([]string, len(n.elements))
	for i, v := range n.elements {
		s[i] = v.String()
	}
	return fmt.Sprintf("[ %s ]", strings.Join(s, ", "))
}

// An ObjectNode represents an object. An object is a list of key-value pairs where the keys are string literals
// and the values are arbitrary nodes.
type ObjectNode struct {
	node

	entries []ObjectPropertyDef
}

// An ObjectPropertyDef represents a property definition in an object.
type ObjectPropertyDef struct {
	Syntax Syntax      // The syntax associated with the property, if any.
	Key    *StringNode // The name of the property.
	Value  Node        // The value of the property.
}

// ObjectPropertySyntax creates a new object property definition with the given key, value, and associated syntax.
func ObjectPropertySyntax(syntax Syntax, key *StringNode, value Node) ObjectPropertyDef {
	return ObjectPropertyDef{
		Syntax: syntax,
		Key:    key,
		Value:  value,
	}
}

// ObjectProperty creates a new object property definition with the given key and value.
func ObjectProperty(key *StringNode, value Node) ObjectPropertyDef {
	value.isNode() // This is a check for a non-nil interface to a nil value.
	return ObjectPropertySyntax(NoSyntax, key, value)
}

// ObjectSyntax creates a new object node with the given properties and associated syntax.
func ObjectSyntax(syntax Syntax, entries ...ObjectPropertyDef) *ObjectNode {
	contract.Assertf(syntax != nil, "Syntax cannot be nil, use NoSyntax instead")
	return &ObjectNode{node: node{syntax: syntax}, entries: entries}
}

// Object creates a new object node with the given properties.
func Object(entries ...ObjectPropertyDef) *ObjectNode {
	return ObjectSyntax(NoSyntax, entries...)
}

// Len returns the number of properties in the object.
func (n *ObjectNode) Len() int {
	return len(n.entries)
}

// Index returns the i'th property of the object.
func (n *ObjectNode) Index(i int) ObjectPropertyDef {
	return n.entries[i]
}

func (n *ObjectNode) String() string {
	if len(n.entries) == 0 {
		return "{ }"
	}
	s := make([]string, len(n.entries))
	for i, v := range n.entries {
		s[i] = v.Key.String() + ": " + v.Value.String()
	}
	return fmt.Sprintf("{ %s }", strings.Join(s, ", "))
}
