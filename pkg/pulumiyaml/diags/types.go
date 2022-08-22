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
	switch t := codegen.UnwrapType(t).(type) {
	case *schema.ObjectType:
		if (adhockObjectToken != "" && strings.HasPrefix(t.Token, adhockObjectToken)) || t.Token == "" {
			// The token is useless so display the fields
			props := []string{}
			for _, prop := range t.Properties {
				props = append(props, fmt.Sprintf("%s: %s", prop.Name,
					DisplayTypeWithAdhock(prop.Type, adhockObjectToken)))
			}
			return fmt.Sprintf("{%s}", strings.Join(props, ", "))
		}
		return t.Token

	case *schema.ArrayType:
		return fmt.Sprintf("List<%s>",
			DisplayTypeWithAdhock(t.ElementType, adhockObjectToken))
	case *schema.MapType:
		return fmt.Sprintf("Map<%s>",
			DisplayTypeWithAdhock(t.ElementType, adhockObjectToken))
	case *schema.UnionType:
		inner := make([]string, len(t.ElementTypes))
		for i, t := range t.ElementTypes {
			inner[i] = DisplayTypeWithAdhock(t, adhockObjectToken)
		}
		return fmt.Sprintf("Union<%s>", strings.Join(inner, ", "))
	case *schema.TokenType:
		underlying := DisplayTypeWithAdhock(schema.AnyType, adhockObjectToken)
		if t.UnderlyingType != nil {
			underlying = DisplayTypeWithAdhock(t.UnderlyingType, adhockObjectToken)
		}
		return fmt.Sprintf("%s<type = %s>", t.Token, underlying)
	default:
		return t.String()
	}
}
