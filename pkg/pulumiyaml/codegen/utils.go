// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2"
	"github.com/iancoleman/strcase"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/zclconf/go-cty/cty"
)

// camel replaces the first contiguous string of upper case runes in the given string with its lower-case equivalent.
func camel(s string) string {
	return strcase.ToLowerCamel(s)
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
		} else {
			builder.WriteRune('_')
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

type BlockSyntax struct {
	Leading  syntax.TriviaList
	Trailing syntax.TriviaList
}

func (b BlockSyntax) convertTrivia(s syntax.TriviaList) string {
	c := strings.TrimSpace(fmt.Sprint(s))
	c = strings.TrimPrefix(c, "//")
	c = strings.TrimSpace(c)
	return c
}

func (b BlockSyntax) Range() *hcl.Range {
	return nil
}

func (b BlockSyntax) HeadComment() string {
	return b.convertTrivia(b.Leading)
}

func (b BlockSyntax) LineComment() string {
	return b.convertTrivia(b.Trailing)
}

func (b BlockSyntax) FootComment() string {
	return ""
}
