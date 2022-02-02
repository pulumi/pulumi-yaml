package pulumiyaml

import (
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
)

// The TagDecoder is responsible for decoding YAML tags that represent calls to builtin functions. Supported tags are:
//
// - !Ref variable, which expands to { "Ref": "variable" }
// - !GetAtt variable.property, which expands to { "Fn::GetAtt": [ "variable", "property" ] }
// - !Sub [ ... ], which expands to { "Fn::Sub": [ ... ] }
// - !Select [ ... ], which expands to { "Fn::Select": [ ... ] }
// - !Join [ ... ], which expands to { "Fn::Join": [ ... ] }
//
// TODO: add support for Fn::Invoke and Fn::StackReference?
var TagDecoder = tagDecoder(0)

type tagDecoder int

func (d tagDecoder) DecodeTag(filename string, n *yaml.Node) (syntax.Node, syntax.Diagnostics, bool) {
	// Then process tags on this node
	switch n.Tag {
	case "!Ref":
		resourceName, diags := encoding.UnmarshalYAMLNode(filename, n, d)
		if diags.HasErrors() {
			return resourceName, diags, true
		}
		return builtin(resourceName.Syntax(), "Ref", resourceName), diags, true
	case "!GetAtt":
		property, diags := encoding.UnmarshalYAMLNode(filename, n, d)
		if diags.HasErrors() {
			return property, diags, true
		}

		s := property.Syntax()

		str, ok := property.(*syntax.StringNode)
		if !ok {
			diags.Extend(syntax.NodeError(property, "the argument to !GetAtt must be a string of the form 'resourceName.propertyName'", ""))
			return property, diags, true
		}
		parts := strings.Split(str.Value(), ".")
		if len(parts) != 2 {
			diags.Extend(syntax.NodeError(property, "the argument to !GetAtt must be a string of the form 'resourceName.propertyName'", ""))
			return property, diags, true
		}

		return builtin(s, "Fn::GetAtt", syntax.ListSyntax(s, syntax.StringSyntax(s, parts[0]), syntax.StringSyntax(s, parts[1]))), diags, true
	case "!Sub", "!Select", "!Join":
		args, diags := encoding.UnmarshalYAMLNode(filename, n, d)
		if diags.HasErrors() {
			return args, diags, true
		}

		return builtin(args.Syntax(), "Fn::"+n.Tag[1:], args), diags, true
	}
	return nil, nil, false
}

func builtin(s syntax.Syntax, name string, args syntax.Node) syntax.Node {
	return syntax.ObjectSyntax(s, syntax.ObjectPropertySyntax(s, syntax.StringSyntax(s, name), args))
}
