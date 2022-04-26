// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

func unimplimented() string {
	panic("Unimplemented")
}

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
func (d DocLanguageHelper) GetMethodResultName(pkg *schema.Package, modName string, r *schema.Resource, m *schema.Method) string {
	return ""
}

func (d DocLanguageHelper) GetLanguageTypeString(pkg *schema.Package, moduleName string, t schema.Type, input bool) string {
	return unimplimented()
}

func (d DocLanguageHelper) GetFunctionName(modName string, f *schema.Function) string {
	return unimplimented()
}

func (d DocLanguageHelper) GetResourceFunctionResultName(modName string, f *schema.Function) string {
	return unimplimented()
}

func (d DocLanguageHelper) GetMethodName(m *schema.Method) string {
	return ""
}

func (d DocLanguageHelper) GetModuleDocLink(pkg *schema.Package, modName string) (string, string) {
	return unimplimented(), unimplimented()
}

func (d DocLanguageHelper) GetDocLinkForResourceType(pkg *schema.Package, moduleName, typeName string) string {
	return unimplimented()
}

func (d DocLanguageHelper) GetDocLinkForPulumiType(pkg *schema.Package, typeName string) string {
	return unimplimented()
}

func (d DocLanguageHelper) GetDocLinkForResourceInputOrOutputType(pkg *schema.Package, moduleName, typeName string, input bool) string {
	return unimplimented()
}

func (d DocLanguageHelper) GetDocLinkForFunctionInputOrOutputType(pkg *schema.Package, moduleName, typeName string, input bool) string {
	return unimplimented()
}
