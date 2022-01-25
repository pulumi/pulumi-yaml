package syntax

// A Node represents a single node in an object tree.
type Node interface {
	Syntax() Syntax

	isNode()
}

type node struct {
	syntax Syntax
}

func (n *node) Syntax() Syntax {
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
	return ObjectPropertySyntax(NoSyntax, key, value)
}

// ObjectSyntax creates a new object node with the given properties and associated syntax.
func ObjectSyntax(syntax Syntax, entries ...ObjectPropertyDef) *ObjectNode {
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
