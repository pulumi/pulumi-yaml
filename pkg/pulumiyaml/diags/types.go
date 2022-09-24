// Copyright 2022, Pulumi Corporation.  All rights reserved.

package diags

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func DisplayType(t schema.Type) string {
	return DisplayTypeWithAdhock(t, "")
}

func DisplayTypeWithAdhock(t schema.Type, adhockObjectToken string) string {
	if schema.IsPrimitiveType(codegen.UnwrapType(t)) {
		switch codegen.UnwrapType(t) {
		case schema.ArchiveType:
			return "archive"
		case schema.AssetType:
			return "asset"
		case schema.JSONType:
			fallthrough
		case schema.AnyType:
			return "any"
		}

		return codegen.UnwrapType(t).String()
	}
	_, optional := t.(*schema.OptionalType)
	var typ string
	switch t := codegen.UnwrapType(t).(type) {
	case *schema.ObjectType:
		if (adhockObjectToken != "" && strings.HasPrefix(t.Token, adhockObjectToken)) || t.Token == "" {
			// The token is useless so display the fields
			props := []string{}
			for _, prop := range t.Properties {
				props = append(props, fmt.Sprintf("%s: %s", prop.Name,
					DisplayTypeWithAdhock(prop.Type, adhockObjectToken)))
			}
			typ = fmt.Sprintf("{%s}", strings.Join(props, ", "))
		} else {
			typ = t.Token
		}
	case *schema.ArrayType:
		typ = fmt.Sprintf("List<%s>",
			DisplayTypeWithAdhock(t.ElementType, adhockObjectToken))
	case *schema.MapType:
		typ = fmt.Sprintf("Map<%s>",
			DisplayTypeWithAdhock(t.ElementType, adhockObjectToken))
	case *schema.UnionType:
		inner := make([]string, len(t.ElementTypes))
		for i, t := range t.ElementTypes {
			inner[i] = DisplayTypeWithAdhock(t, adhockObjectToken)
		}
		typ = fmt.Sprintf("Union<%s>", strings.Join(inner, ", "))
	case *schema.TokenType:
		underlying := DisplayTypeWithAdhock(schema.AnyType, adhockObjectToken)
		if t.UnderlyingType != nil {
			underlying = DisplayTypeWithAdhock(t.UnderlyingType, adhockObjectToken)
		}
		typ = fmt.Sprintf("%s<type = %s>", t.Token, underlying)
	default:
		typ = t.String()
	}
	if optional {
		typ += "?"
	}
	return typ
}
