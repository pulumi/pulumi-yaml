// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

// The TagDecoder is responsible for decoding YAML tags that represent calls to builtin functions.
//
// No tags are presently supported, but the machinery to support tags is useful to preserve until
// we are confident that we won't re-introduce.
var TagDecoder = tagDecoder(0)

type tagDecoder int

func (d tagDecoder) DecodeTag(filename string, n *yaml.Node) (syntax.Node, syntax.Diagnostics, bool) {
	return nil, nil, false
}
