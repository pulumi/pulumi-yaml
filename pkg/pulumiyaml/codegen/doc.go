// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// DocLanguageHelper is the YAML-specific implementation of the DocLanguageHelper.
type DocLanguageHelper struct{}

func (d DocLanguageHelper) GetPropertyName(p *schema.Property) (string, error) {
	return p.Name, nil
}

// Pulumi YAML doesn't have enums, so you should just use the value itself.
func (d DocLanguageHelper) GetEnumName(e *schema.Enum, typeName string) (string, error) {
	return fmt.Sprintf("%q", e.Value), nil
}

// There is no way to name types besides resources and invokes in Pulumi YAML.
func (d DocLanguageHelper) GetMethodResultName(schema.PackageReference, string, *schema.Resource, *schema.Method) string {
	return ""
}

func (d DocLanguageHelper) GetModuleName(_ schema.PackageReference, modName string) string {
	if modName == "index" {
		return ""
	}
	return modName
}

func (d DocLanguageHelper) GetTypeName(pkg schema.PackageReference, t schema.Type, input bool, relativeTo string) string {
	getType := func(t schema.Type) string {
		return d.GetTypeName(pkg, t, input, relativeTo)
	}
	if schema.IsPrimitiveType(t) {
		switch t {
		case schema.NumberType, schema.IntType:
			return "Number"
		case schema.StringType:
			return "String"
		case schema.BoolType:
			return "Boolean"
		case schema.ArchiveType:
			return "Archive"
		case schema.AssetType:
			return "Asset"
		case schema.JSONType:
			return "JSON"
		case schema.AnyType:
			return "Any"
		}
	}
	switch t := t.(type) {
	case *schema.ResourceType:
		return collapseToken(t.Token)
	case *schema.ArrayType:
		return fmt.Sprintf("List<%s>", getType(t.ElementType))
	case *schema.InputType:
		return getType(t.ElementType)
	case *schema.MapType:
		return fmt.Sprintf("Map<%s>", getType(t.ElementType))
	case *schema.UnionType:
		if len(t.ElementTypes) == 0 {
			return ""
		}
		types := getType(t.ElementTypes[0])
		for i := 1; i < len(t.ElementTypes); i++ {
			types += " | " + getType(t.ElementTypes[i])
		}
		return types
	case *schema.EnumType:
		if len(t.Elements) == 0 {
			return ""
		}
		toString := func(v interface{}) string {
			switch v := v.(type) {
			case string:
				return fmt.Sprintf("%q", v)
			default:
				return fmt.Sprintf("%v", v)
			}
		}
		values := toString(t.Elements[0].Value)
		for i := 1; i < len(t.Elements); i++ {
			values += " | " + toString(t.Elements[i].Value)
		}
		return values
	case *schema.OptionalType:
		return getType(t.ElementType)
	case *schema.ObjectType:
		return "Property Map"
	default:
		return ""
	}
}

func (d DocLanguageHelper) GetFunctionName(f *schema.Function) string {
	return collapseToken(f.Token)
}

func (d DocLanguageHelper) GetResourceName(r *schema.Resource) string {
	return collapseToken(r.Token)
}

func (d DocLanguageHelper) GetResourceFunctionResultName(modName string, f *schema.Function) string {
	return ""
}

func (d DocLanguageHelper) GetMethodName(m *schema.Method) string {
	return ""
}

// Doc links

func (d DocLanguageHelper) GetModuleDocLink(pkg *schema.Package, modName string) (string, string) {
	return fmt.Sprintf("%s:%s", pkg.Name, modName), ""
}

func (d DocLanguageHelper) GetDocLinkForResourceType(pkg *schema.Package, moduleName, typeName string) string {
	return ""
}

func (d DocLanguageHelper) GetDocLinkForPulumiType(pkg *schema.Package, typeName string) string {
	return ""
}

func (d DocLanguageHelper) GetDocLinkForResourceInputOrOutputType(pkg *schema.Package, moduleName, typeName string, input bool) string {
	return ""
}

func (d DocLanguageHelper) GetDocLinkForFunctionInputOrOutputType(pkg *schema.Package, moduleName, typeName string, input bool) string {
	return ""
}
