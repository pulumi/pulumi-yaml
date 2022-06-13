// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypeError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from, to schema.Type
		message  string
	}{
		{
			from: &schema.UnionType{
				ElementTypes: []schema.Type{
					schema.StringType,
					schema.NumberType,
				},
			},
			to: schema.NumberType,
			message: `Cannot assign 'Union<string, number>' to 'number':
  Cannot assign type 'string' to type 'number'`,
		},
		{
			from: &schema.UnionType{
				ElementTypes: []schema.Type{
					schema.StringType,
					schema.NumberType,
				},
			},
			to: schema.AnyType,
		},
		{
			from: &schema.ResourceType{
				Token:    "some:resource:Token",
				Resource: &schema.Resource{},
			},
			// Empty resource type accepts all resources
			to: &schema.ResourceType{
				Token: "some:other:Token",
			},
			message: "Cannot assign 'some:resource:Token' to 'some:other:Token'",
		},
		{
			from: &schema.ArrayType{ElementType: &schema.ObjectType{
				Properties: []*schema.Property{
					{Name: "foo", Type: schema.StringType},
					{Name: "bar", Type: schema.NumberType},
				},
			}},
			to: &schema.ArrayType{ElementType: &schema.MapType{ElementType: schema.StringType}},
		},
		{
			from: &schema.ArrayType{ElementType: &schema.ObjectType{
				Token: adhockObjectToken + "Token",
				Properties: []*schema.Property{
					{Name: "foo", Type: schema.StringType},
					{Name: "bar", Type: schema.AnyType},
				},
			}},
			to: &schema.ArrayType{ElementType: &schema.MapType{ElementType: schema.StringType}},
			message: `Cannot assign 'List<{foo: string, bar: any}>' to 'List<Map<string>>':
  Cannot assign '{foo: string, bar: any}' to 'Map<string>':
    bar: Cannot assign type 'any' to type 'string'`,
		},
		{
			from: &schema.ObjectType{
				Token: adhockObjectToken + "Token",
				Properties: []*schema.Property{
					{Name: "prop1", Type: schema.ArchiveType},
					{Name: "prop2", Type: schema.BoolType},
				},
			},
			to: &schema.ObjectType{
				Token: adhockObjectToken + "Token2",
				Properties: []*schema.Property{
					{Name: "prop1", Type: schema.AssetType},
					{Name: "prop2", Type: schema.StringType},
					{Name: "optional", Type: &schema.OptionalType{ElementType: schema.AnyType}},
				},
			},
		},
		{
			from: &schema.ObjectType{
				Token: adhockObjectToken + "Token",
				Properties: []*schema.Property{
					{Name: "prop1", Type: schema.AssetType},
					{Name: "prop3", Type: schema.AnyType},
				},
			},
			to: &schema.ObjectType{
				Token: adhockObjectToken + "Token2",
				Properties: []*schema.Property{
					{Name: "prop1", Type: schema.ArchiveType},
					{Name: "prop2", Type: schema.BoolType},
					{Name: "prop3", Type: &schema.OptionalType{ElementType: schema.StringType}},
				},
			},
			message: `Cannot assign '{prop1: asset, prop3: any}' to '{prop1: archive, prop2: boolean, prop3: string}':
  prop1: Cannot assign type 'asset' to type 'archive'
  prop2: Missing required property 'prop2'
  prop3: Cannot assign type 'any' to type 'string'`,
		},

		// Token Types:
		{
			// Can assign between token types with compatible underlying types.
			from: &schema.TokenType{Token: "foo:bar:baz", UnderlyingType: schema.NumberType},
			to:   &schema.TokenType{Token: "foo:fizz:buzz", UnderlyingType: schema.StringType},
		},
		{
			// Token types are assignable to the 'any' type
			from: &schema.TokenType{Token: "foo"},
			to:   schema.AnyType,
		},
		{
			// Token types are assignable to the 'any' type, and no other type
			from: &schema.TokenType{Token: "foo"},
			to:   schema.StringType,
			message: `Cannot assign 'foo<type = any>' to 'string'. 'foo<type = any>' is a Token Type. Token types act like their underlying type:
  Cannot assign type 'any' to type 'string'`,
		},
		{
			// Token types are assignable to their underlying types
			from: &schema.TokenType{Token: "tk:index:Tk", UnderlyingType: schema.StringType},
			to:   schema.StringType,
		},
		{
			// Token types are assignable to compatible underlying types
			from: &schema.TokenType{Token: "tk:index:Tk", UnderlyingType: schema.BoolType},
			to:   schema.StringType,
		},
		{
			// You can assign into token types from compatible plain types
			from: schema.BoolType,
			to:   &schema.TokenType{Token: "tk:index:Tk", UnderlyingType: schema.StringType},
		},
		{
			// You can assign into token types the underlying type
			from: schema.BoolType,
			to:   &schema.TokenType{Token: "tk:index:Tk", UnderlyingType: schema.BoolType},
		},
	}

	for i, c := range cases { //nolint:paralleltest
		// false positive. The parallel call is below
		c := c

		name := c.message
		if name == "" {
			name = fmt.Sprintf("no-error%d", i)
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := isAssignable(c.from, c.to)
			if c.message == "" {
				assert.Nil(t, result)
				if t.Failed() {
					t.Logf("err: %s", result.Error())
				}
			} else {
				require.Error(t, result)
				assert.Equal(t, c.message, result.String())
			}
		})
	}
}

func TestTypePropertyAccess(t *testing.T) {
	t.Parallel()
	cases := []struct {
		root         schema.Type
		list         []ast.PropertyAccessor
		expectedType string
		errMsg       string
	}{
		{
			root: &schema.MapType{ElementType: &schema.ArrayType{ElementType: schema.AnyType}},
			list: []ast.PropertyAccessor{
				&ast.PropertySubscript{Index: "foo"},
				&ast.PropertySubscript{Index: 7},
				&ast.PropertySubscript{Index: "foo"},
			},
			expectedType: "Invalid",
			errMsg:       `Cannot index into 'start["foo"][7]' (type any):Index property access is only allowed on Maps and Lists`,
		},
		{
			root: &schema.ResourceType{
				Token: "pkg:mod:Token",
				Resource: &schema.Resource{
					Properties: []*schema.Property{
						{Name: "fizz", Type: schema.StringType},
						{Name: "buzz", Type: schema.StringType},
					},
				},
			},
			list: []ast.PropertyAccessor{
				&ast.PropertyName{Name: "fizzbuzz"},
			},
			expectedType: "Invalid",
			errMsg:       `fizzbuzz does not exist on start:Existing properties are: buzz, fizz, id, urn`,
		},
		{
			root: &schema.UnionType{
				ElementTypes: []schema.Type{
					&schema.ArrayType{ElementType: schema.StringType},
					&schema.ArrayType{ElementType: schema.NumberType},
				},
			},
			list: []ast.PropertyAccessor{
				&ast.PropertySubscript{Index: 0},
			},
			expectedType: "Union<string, number>",
			errMsg:       ``,
		},
		{
			root: &schema.UnionType{
				ElementTypes: []schema.Type{
					&schema.ArrayType{ElementType: schema.StringType},
					&schema.MapType{ElementType: schema.NumberType},
					&schema.ObjectType{
						Properties: []*schema.Property{
							{Name: "foo", Type: &schema.ArrayType{ElementType: schema.AnyType}},
						},
					},
				},
			},
			list: []ast.PropertyAccessor{
				&ast.PropertyName{Name: "foo"},
				&ast.PropertySubscript{Index: "bar"},
			},
			expectedType: "Invalid",
			errMsg: `Cannot access into start of type Union<List<string>, Map<number>, >:'start' could be a type that does not support accessing:
  Array<string>: cannot access a property on 'start' (type List<string>)
  Map<number>: cannot access a property on 'start' (type Map<number>)
  Cannot index via string into 'start.foo' (type List<any>)`,
		},
	}

	for _, c := range cases { //nolint:paralleltest
		// false positive. The parallel call is below

		c := c
		t.Run("", func(t *testing.T) {
			t.Parallel()
			var actualMsg string
			setError := func(m, s string) *schema.InvalidType {
				actualMsg += m + ":" + s + "\n"
				return &schema.InvalidType{}
			}
			actualType := typePropertyAccess(nil, c.root, "start", c.list, setError)
			assert.Equal(t, c.expectedType, displayType(actualType))
			assert.Equal(t, c.errMsg, strings.TrimSuffix(actualMsg, "\n"))
		})
	}
}
