// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
)

// The TagDecoder is responsible for decoding YAML tags that represent calls to builtin functions. Supported tags are:
//
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
