// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
)

func TestGetResourceName(t *testing.T) {
	t.Parallel()

	var helper DocLanguageHelper

	assert.Equal(t, "pkg:Resource", helper.GetResourceName(&schema.Resource{
		Token: "pkg:index:Resource",
	}))
	assert.Equal(t, "pkg:complex:Token", helper.GetResourceName(&schema.Resource{
		Token: "pkg:complex:Token",
	}))
	assert.Equal(t, "pkg:mod:ResToken", helper.GetResourceName(&schema.Resource{
		Token: "pkg:mod/resToken:ResToken",
	}))
}

func TestGetModuleName(t *testing.T) {
	t.Parallel()

	var helper DocLanguageHelper

	assert.Equal(t, "", helper.GetModuleName(nil, "index"))
	assert.Equal(t, "foo", helper.GetModuleName(nil, "foo"))
}

func TestActuallDocLanguageHelper(t *testing.T) {
	t.Parallel()
	func(codegen.DocLanguageHelper) {}(DocLanguageHelper{})
}

func TestResolveDocRef(t *testing.T) {
	t.Parallel()

	helper := DocLanguageHelper{}

	tests := []struct {
		name     string
		ref      schema.DocRef
		expected string
		resolved bool
	}{
		{
			name: "resource",
			ref: schema.DocRef{
				Kind: schema.DocRefKindResource,
				Type: &schema.ResourceType{Token: "pkg:mod/resToken:ResToken"},
			},
			expected: "pkg:mod:ResToken",
			resolved: true,
		},
		{
			name: "function",
			ref: schema.DocRef{
				Kind:     schema.DocRefKindFunction,
				Function: &schema.Function{Token: "pkg:index:getThing"},
			},
			expected: "pkg:getThing",
			resolved: true,
		},
		{
			name: "resource property",
			ref: schema.DocRef{
				Kind:     schema.DocRefKindResourceProperty,
				Property: "someProp",
			},
			expected: "someProp",
			resolved: true,
		},
		{
			name: "type",
			ref: schema.DocRef{
				Kind: schema.DocRefKindType,
				Type: &schema.ObjectType{Token: "pkg:mod/type:Type"},
			},
			expected: "",
			resolved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, ok, err := helper.ResolveDocRef(nil, schema.DocRef{}, tt.ref)
			assert.NoError(t, err)
			assert.Equal(t, tt.resolved, ok)
			assert.Equal(t, tt.expected, name)
		})
	}
}

func TestGetTypeName(t *testing.T) {
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
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			result := helper.GetTypeName(nil, tt.inputType, true, "no-mode")
			assert.Equal(t, tt.expected, result)
		})
	}
}
