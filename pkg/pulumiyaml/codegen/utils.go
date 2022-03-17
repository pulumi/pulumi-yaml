// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/zclconf/go-cty/cty"
)

// camel replaces the first contiguous string of upper case runes in the given string with its lower-case equivalent.
func camel(s string) string {
	c, sz := utf8.DecodeRuneInString(s)
	if sz == 0 || unicode.IsLower(c) {
		return s
	}

	// The first rune is not lowercase. Iterate until we find a rune that is.
	var word []rune
	for {
		s = s[sz:]

		n, nsz := utf8.DecodeRuneInString(s)
		if nsz == 0 {
			word = append(word, unicode.ToLower(c))
			return string(word)
		}
		if unicode.IsLower(n) {
			if len(word) == 0 {
				c = unicode.ToLower(c)
			}
			word = append(word, c)
			return string(word) + s
		}
		c, sz, word = n, nsz, append(word, unicode.ToLower(c))
	}
}

// isLegalIdentifierStart returns true if it is legal for c to be the first character of an HCL2 identifier.
func isLegalIdentifierStart(c rune) bool {
	return c == '$' || c == '_' ||
		unicode.In(c, unicode.Lu, unicode.Ll, unicode.Lt, unicode.Lm, unicode.Lo, unicode.Nl)
}

// isLegalIdentifierPart returns true if it is legal for c to be part of an HCL2 identifier.
func isLegalIdentifierPart(c rune) bool {
	return isLegalIdentifierStart(c) || unicode.In(c, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc)
}

// makeLegalIdentifier deletes characters that are not allowed in HCL2 identifiers with underscores. No attempt is
// made to ensure that the result is unique.
func makeLegalIdentifier(name string) string {
	var builder strings.Builder
	for i, c := range name {
		if isLegalIdentifierPart(c) {
			if i == 0 && !isLegalIdentifierStart(c) {
				builder.WriteRune('_')
			}
			builder.WriteRune(c)
		}
	}
	if builder.Len() == 0 {
		return "x"
	}
	return builder.String()
}

// plainLit returns an unquoted string literal expression.
func plainLit(v string) *model.LiteralValueExpression {
	return &model.LiteralValueExpression{Value: cty.StringVal(v)}
}

// quotedLit returns a quoted string literal expression.
func quotedLit(v string) *model.TemplateExpression {
	return &model.TemplateExpression{Parts: []model.Expression{plainLit(v)}}
}

// relativeTraversal returns a new RelativeTraversalExpression that accesses the given attribute of the source
// expression.
func relativeTraversal(source model.Expression, attr string) *model.RelativeTraversalExpression {
	return &model.RelativeTraversalExpression{
		Source:    source,
		Traversal: hcl.Traversal{hcl.TraverseAttr{Name: attr}},
		Parts:     []model.Traversable{model.DynamicType, model.DynamicType},
	}
}

// resourceToken returns the Pulumi token for the given CloudFormation resource type.
func resourceToken(typ string) string {
	components := strings.Split(typ, "::")
	if len(components) != 3 {
		return normalizeType(typ)
	}
	moduleName, resourceName := components[1], components[2]

	// Override the name of the Config module.
	if moduleName == "Config" {
		moduleName = "Configuration"
	}
	return "cloudformation:" + moduleName + ":" + resourceName
}

func normalizeType(typ string) string {
	if parts := strings.Split(typ, ":"); len(parts) == 2 {
		typ = fmt.Sprintf("%s:index:%s", parts[0], parts[1])
	}
	return typ
}
