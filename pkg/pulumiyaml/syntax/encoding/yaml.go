// Copyright 2022, Pulumi Corporation.  All rights reserved.

package encoding

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

// A TagDecoder decodes tagged YAML nodes. See the documentation on UnmarshalYAML for more details.
type TagDecoder interface {
	// DecodeTag decodes a tagged YAML node.
	DecodeTag(filename string, n *yaml.Node) (syntax.Node, syntax.Diagnostics, bool)
}

// YAMLSyntax is a syntax.Syntax implementation that is backed by a YAML node.
type YAMLSyntax struct {
	*yaml.Node
	rng   *hcl.Range
	value interface{}
}

// Range returns the textual range of the YAML node, if any.
func (s YAMLSyntax) Range() *hcl.Range {
	return s.rng
}

// yamlEndPos calculates the end position of a YAML node.
//
// For simple scalars, this is reasonably accurate: the end position is (start line + the number of lines, start
// column + the length of the last line).
//
// For sequences and mappings, the end position of the last node in the sequence or mapping is used as the end position
// of the sequence or mapping itself. This works well for block-style sequences/mappings, but misses the closing token
// for flow-style sequences/mappings.
func yamlEndPos(n *yaml.Node) hcl.Pos {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode, yaml.MappingNode:
		if len(n.Content) != 0 {
			return yamlEndPos(n.Content[len(n.Content)-1])
		}
		return hcl.Pos{Line: n.Line, Column: n.Column}
	default:
		line, col, s := n.Line, n.Column, n.Value
		switch n.Style {
		case yaml.LiteralStyle:
			for {
				nl := strings.IndexByte(s, '\n')
				if nl == -1 {
					break
				}
				line, s = line+1, s[nl+1:]
			}
		case yaml.TaggedStyle:
			col += len(n.Tag) + 1
		}
		return hcl.Pos{Line: line, Column: col + len(s)}
	}
}

// yamlNodeRange returns a Range for the given YAML node.
func yamlNodeRange(filename string, n *yaml.Node) *hcl.Range {
	startPos := hcl.Pos{Line: n.Line, Column: n.Column}
	endPos := yamlEndPos(n)
	return &hcl.Range{Filename: filename, Start: startPos, End: endPos}
}

// UnmarshalYAMLNode unmarshals the given YAML node into a syntax Node. UnmarshalYAMLNode does _not_ use the tag decoder
// for the node itself, though it does use the tag decoder for the node's children. This allows tag decoders to call
// UnmarshalYAMLNode without infinitely recurring on the same node. See UnmarshalYAML for more details.
func UnmarshalYAMLNode(filename string, n *yaml.Node, tags TagDecoder) (syntax.Node, syntax.Diagnostics) {
	rng := yamlNodeRange(filename, n)

	var diags syntax.Diagnostics
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		var elements []syntax.Node
		if len(n.Content) != 0 {
			elements = make([]syntax.Node, len(n.Content))
			for i, v := range n.Content {
				e, ediags := UnmarshalYAML(filename, v, tags)
				diags.Extend(ediags...)

				elements[i] = e
			}
		}
		return syntax.ListSyntax(YAMLSyntax{n, rng, nil}, elements...), diags
	case yaml.MappingNode:
		var entries []syntax.ObjectPropertyDef
		if len(n.Content) != 0 {
			// mappings are represented as a sequence of the form [key_0, value_0, ... key_n, value_n]
			numEntries := len(n.Content) / 2
			entries = make([]syntax.ObjectPropertyDef, numEntries)
			for i := range entries {
				keyNode, valueNode := n.Content[2*i], n.Content[2*i+1]

				keyn, kdiags := UnmarshalYAML(filename, keyNode, tags)
				diags.Extend(kdiags...)

				key, ok := keyn.(*syntax.StringNode)
				if !ok {
					keyRange := keyn.Syntax().Range()
					diags.Extend(syntax.Error(keyRange, "mapping keys must be strings", ""))
				}

				value, vdiags := UnmarshalYAML(filename, valueNode, tags)
				diags.Extend(vdiags...)

				entries[i] = syntax.ObjectPropertySyntax(YAMLSyntax{keyNode, rng, nil}, key, value)
			}
		}
		return syntax.ObjectSyntax(YAMLSyntax{n, rng, nil}, entries...), diags
	case yaml.ScalarNode:
		var v interface{}
		if err := n.Decode(&v); err != nil {
			diags.Extend(syntax.Error(rng, err.Error(), ""))
			return nil, diags
		}
		if v == nil {
			return syntax.NullSyntax(YAMLSyntax{n, rng, nil}), nil
		}

		switch v := v.(type) {
		case bool:
			return syntax.BooleanSyntax(YAMLSyntax{n, rng, v}, v), nil
		case float64:
			return syntax.NumberSyntax(YAMLSyntax{n, rng, v}, v), nil
		case int:
			return syntax.NumberSyntax(YAMLSyntax{n, rng, float64(v)}, float64(v)), nil
		case int64:
			return syntax.NumberSyntax(YAMLSyntax{n, rng, float64(v)}, float64(v)), nil
		case uint64:
			return syntax.NumberSyntax(YAMLSyntax{n, rng, float64(v)}, float64(v)), nil
		default:
			return syntax.StringSyntax(YAMLSyntax{n, rng, v}, n.Value), nil
		}
	case yaml.AliasNode:
		return nil, syntax.Diagnostics{syntax.Error(rng, "alias nodes are not supported", "")}
	default:
		return nil, syntax.Diagnostics{syntax.Error(rng, fmt.Sprintf("unexpected node kind %v", n.Kind), "")}
	}
}

// UnmarshalYAML unmarshals a YAML node into a syntax node.
//
// Nodes are decoded as follows:
// - Scalars are decoded as the corresponding literal type (null -> nullNode, bool -> BooleanNode, etc.)
// - Sequences are decoded as array nodes
// - Mappings are decoded as object nodes
//
// Tagged nodes are decoded using the given TagDecoder. To avoid infinite recursion, the TagDecoder must call
// UnmarshalYAMLNode if it needs to unmarshal the node it is processing.
func UnmarshalYAML(filename string, n *yaml.Node, tags TagDecoder) (syntax.Node, syntax.Diagnostics) {
	if tags != nil {
		if s, diags, ok := tags.DecodeTag(filename, n); ok {
			return s, diags
		}
	}
	return UnmarshalYAMLNode(filename, n, tags)
}

// MarshalYAML marshals a syntax node into a YAML node. If a syntax node has an associated YAMLSyntax annotation,
// the tag, style, and comments on the result will be pulled from the YAMLSyntax. The marshaling process otherwise
// follows the inverse of the unmarshaling process described in the documentation for UnmarshalYAML.
func MarshalYAML(n syntax.Node) (*yaml.Node, syntax.Diagnostics) {
	var yamlNode yaml.Node
	var originalValue interface{}
	if original, ok := n.Syntax().(YAMLSyntax); ok {
		yamlNode.Tag = original.Tag
		yamlNode.Value = original.Value
		yamlNode.Style = original.Style
		yamlNode.HeadComment = original.HeadComment
		yamlNode.LineComment = original.LineComment
		yamlNode.FootComment = original.FootComment

		originalValue = original.value
	}

	stringNode := func(value string) {
		yamlNode.Kind = yaml.ScalarNode
		if yamlNode.Tag != "" && yamlNode.Tag != "!!str" {
			yamlNode.Tag = "!!str"
		}
		if originalValue != value {
			yamlNode.Value = value
		}
	}

	var diags syntax.Diagnostics
	switch n := n.(type) {
	case *syntax.NullNode:
		yamlNode.Kind = yaml.ScalarNode
		if yamlNode.Tag != "" && yamlNode.Tag != "!!null" {
			yamlNode.Tag = "!!null"
		}
		switch yamlNode.Value {
		case "null", "Null", "NULL", "~":
			// OK
		default:
			yamlNode.Value = "null"
		}
	case *syntax.BooleanNode:
		yamlNode.Kind = yaml.ScalarNode
		if yamlNode.Tag != "" && yamlNode.Tag != "!!bool" {
			yamlNode.Tag = "!!bool"
		}
		if originalValue != n.Value() {
			yamlNode.Value = strconv.FormatBool(n.Value())
		}
	case *syntax.NumberNode:
		yamlNode.Kind = yaml.ScalarNode
		if yamlNode.Tag != "" && yamlNode.Tag != "!!int" && yamlNode.Tag != "!!float" {
			yamlNode.Tag = "!!float"
		}
		if originalValue != n.Value() {
			if yamlNode.Tag == "!!int" {
				yamlNode.Value = strconv.FormatInt(int64(n.Value()), 10)
			} else {
				yamlNode.Value = strconv.FormatFloat(n.Value(), 'g', -1, 64)
			}
		}
	case *syntax.StringNode:
		stringNode(n.Value())
	case *syntax.ListNode:
		if yamlNode.Kind != yaml.SequenceNode && yamlNode.Kind != yaml.DocumentNode {
			yamlNode.Kind = yaml.SequenceNode
		}

		var content []*yaml.Node
		if n.Len() != 0 {
			content = make([]*yaml.Node, n.Len())
			for i := range content {
				e, ediags := MarshalYAML(n.Index(i))
				diags.Extend(ediags...)

				content[i] = e
			}
		}
		yamlNode.Content = content
	case *syntax.ObjectNode:
		yamlNode.Kind = yaml.MappingNode

		var content []*yaml.Node
		if n.Len() != 0 {
			content = make([]*yaml.Node, 2*n.Len())
			for i := 0; i < n.Len(); i++ {
				kvp := n.Index(i)

				k, kdiags := MarshalYAML(kvp.Key)
				diags.Extend(kdiags...)

				v, vdiags := MarshalYAML(kvp.Value)
				diags.Extend(vdiags...)

				content[2*i], content[2*i+1] = k, v
			}
		}
		yamlNode.Content = content
	}

	return &yamlNode, diags
}

type yamlValue struct {
	filename string
	node     syntax.Node
	tags     TagDecoder
	diags    syntax.Diagnostics
}

func (v *yamlValue) UnmarshalYAML(n *yaml.Node) error {
	v.node, v.diags = UnmarshalYAML(v.filename, n, v.tags)
	return nil
}

// DecodeYAML decodes a YAML value from the given decoder into a syntax node. See UnmarshalYAML for mode details on the
// decoding process.
func DecodeYAML(filename string, d *yaml.Decoder, tags TagDecoder) (*syntax.ObjectNode, syntax.Diagnostics) {
	v := yamlValue{filename: filename, tags: tags}
	if err := d.Decode(&v); err != nil {
		return nil, syntax.Diagnostics{syntax.Error(nil, err.Error(), "")}
	}
	obj, ok := v.node.(*syntax.ObjectNode)
	if !ok {
		return nil, syntax.Diagnostics{syntax.Error(nil,
			fmt.Sprintf("Top level of '%s' must be an object", filename), "")}
	}
	return obj, v.diags
}

// EncodeYAML encodes a syntax node into YAML text using the given encoder. See MarshalYAML for mode details on the
// encoding process.
func EncodeYAML(e *yaml.Encoder, n syntax.Node) syntax.Diagnostics {
	yamlNode, diags := MarshalYAML(n)
	if err := e.Encode(yamlNode); err != nil {
		diags.Extend(syntax.Error(nil, err.Error(), ""))
	}
	return diags
}
