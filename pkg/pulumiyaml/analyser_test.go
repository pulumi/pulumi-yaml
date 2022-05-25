package pulumiyaml

import (
	"fmt"
	"testing"

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
