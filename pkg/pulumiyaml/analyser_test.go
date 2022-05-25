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
			message: `Cannot assign Union<string, number> to type number:
  Cannot assign string to type number`,
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
			message: "Cannot assign some:resource:Token to type some:other:Token",
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
			message: `Cannot assign List<{foo: string, bar: pulumi:pulumi:Any}> to type List<Map<string>>:
  Cannot assign {foo: string, bar: pulumi:pulumi:Any} to type Map<string>:
    bar: Cannot assign pulumi:pulumi:Any to type string`,
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
			message: `Cannot assign {prop1: pulumi:pulumi:Asset, prop3: pulumi:pulumi:Any} to type {prop1: pulumi:pulumi:Archive, prop2: boolean, prop3: string}:
  prop1: Cannot assign pulumi:pulumi:Asset to type pulumi:pulumi:Archive
  prop2: Missing required property 'prop2'
  prop3: Cannot assign pulumi:pulumi:Any to type string`,
		},
	}

	for i, c := range cases {
		c := c

		name := c.message
		if name == "" {
			name = fmt.Sprintf("no-error%d", i)
		}
		t.Run(name, func(t *testing.T) {
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
			errMsg:       `Cannot index into 'start["foo"][7]' (type pulumi:pulumi:Any):Index property access is only allowed on Maps and Lists`,
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
  Cannot index via string into 'start.foo' (type List<pulumi:pulumi:Any>)`,
		},
	}

	for _, c := range cases {
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
