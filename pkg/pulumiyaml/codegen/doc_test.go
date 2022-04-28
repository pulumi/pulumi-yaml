// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
)

func TestActuallDocLanguageHelper(t *testing.T) {
	t.Parallel()
	func(codegen.DocLanguageHelper) {}(DocLanguageHelper{})
}

func TestGetLangauageTypeString(t *testing.T) {
	t.Parallel()
	helper := DocLanguageHelper{}

	tests := []struct {
		inputType schema.Type
		expected  string
	}{
		{&schema.MapType{ElementType: schema.StringType}, "Map<String>"},
		{&schema.OptionalType{ElementType: &schema.ArrayType{ElementType: schema.BoolType}}, "List<Boolean>"},
		{&schema.UnionType{ElementTypes: []schema.Type{
			schema.StringType,
			&schema.ArrayType{ElementType: schema.AssetType},
		}}, "String | List<Asset>"},
		{&schema.UnionType{ElementTypes: []schema.Type{schema.NumberType}}, "Number"},
		{&schema.UnionType{ElementTypes: []schema.Type{}}, ""},
		{&schema.EnumType{Elements: []*schema.Enum{
			{Value: "foo"},
			{Value: "Bar"},
		}}, `"foo" | "Bar"`},
		{&schema.EnumType{Elements: []*schema.Enum{{Value: 3.8}}}, "3.8"},
		{&schema.EnumType{Elements: []*schema.Enum{}}, ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			result := helper.GetLanguageTypeString(nil, "no-mode", tt.inputType, true)
			assert.Equal(t, tt.expected, result)
		})
	}
}
