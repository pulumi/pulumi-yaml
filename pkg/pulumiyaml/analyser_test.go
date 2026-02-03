// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypeError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from, to schema.Type
		fromExpr ast.Expr
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
  prop2: Missing required property 'prop2'`,
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
			from:    &schema.TokenType{Token: "foo"},
			to:      schema.StringType,
			message: `Cannot assign 'foo<type = any>' to 'string'`,
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
		{
			from:     schema.StringType,
			fromExpr: ast.String("notValid"),
			to: &schema.EnumType{
				Token: "tk:index:Enum",
				Elements: []*schema.Enum{{
					Name:  "fizz",
					Value: "foo",
				}, {
					Value: "bar",
				}},
				ElementType: schema.StringType,
			},
			message: `Cannot assign type 'string' to type 'tk:index:Enum':
  Allowed values are fizz ("foo"), "bar"`,
		},
		{
			from:     schema.NumberType,
			fromExpr: ast.Number(0.55),
			to: &schema.EnumType{
				Token: "tk:index:Enum",
				Elements: []*schema.Enum{{
					Name:  "fizz",
					Value: 0.0,
				}, {
					Value: 0.5,
				}, {
					Value: 1.0,
				}},
				ElementType: schema.StringType,
			},
			message: `Cannot assign type 'number' to type 'tk:index:Enum':
  Allowed values are fizz (0), 0.5, 1`,
		},
	}

	for i, c := range cases { //nolint:paralleltest
		// false positive. The parallel call is below

		name := c.message
		if name == "" {
			name = fmt.Sprintf("no-error%d", i)
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			expr := c.fromExpr
			if expr == nil {
				expr = ast.StringSyntax(syntax.String("standin"))
			}
			tc := typeCache{
				exprs: map[ast.Expr]schema.Type{
					expr: c.from,
				},
			}
			result := tc.isAssignable(expr, c.to)
			if c.message == "" {
				assert.Nil(t, result)
				if t.Failed() {
					t.Logf("err: %s", result.Error())
				}
			} else {
				require.Error(t, result, fmt.Sprintf("Expected error %q, no error", c.message))
				if result != nil {
					assert.Equal(t, c.message, result.String())
				}
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
			expectedType: "any",
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
			errMsg: `Cannot access into start of type Union<List<string>, Map<number>, {foo: List<any>}>:'start' could be a type that does not support accessing:
  Array<string>: cannot access a property on 'start' (type List<string>)
  Cannot index via string into 'start.foo' (type List<any>)`,
		},
	}

	for _, c := range cases { //nolint:paralleltest
		// false positive. The parallel call is below

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

// tests for type compatibility, i.e. int&number are compatible, int&string are not
func TestConfigCompatibility(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typeA      schema.Type
		typeB      schema.Type
		valB       interface{}
		compatible bool
	}{
		{
			typeA:      schema.IntType,
			typeB:      schema.IntType,
			compatible: true,
		},
		{
			typeA:      schema.IntType,
			typeB:      schema.NumberType,
			compatible: false,
		},
		{
			typeA:      schema.NumberType,
			typeB:      schema.IntType,
			compatible: true,
		},
		{
			typeA:      schema.IntType,
			typeB:      schema.BoolType,
			compatible: false,
		},
		{
			typeA:      schema.StringType,
			typeB:      schema.AnyType,
			compatible: true,
		},
		{
			typeA:      schema.IntType,
			typeB:      schema.StringType,
			valB:       "10",
			compatible: true,
		},
		{
			typeA:      schema.IntType,
			typeB:      schema.StringType,
			valB:       "foo",
			compatible: false,
		},
	}

	for _, c := range cases { //nolint:paralleltest
		// false positive. The parallel call is below

		t.Run("", func(t *testing.T) {
			t.Parallel()
			ok := isTypeCompatible(c.typeA, c.typeB, c.valB)
			assert.Equal(t, c.compatible, ok)
		})
	}
}

func TestNonStringKeyInObjectReturnsError(t *testing.T) {
	t.Parallel()

	tc := typeCache{
		exprs: make(map[ast.Expr]schema.Type),
	}
	expr := &ast.ObjectExpr{
		Entries: []ast.ObjectProperty{
			{
				Key: &ast.BooleanExpr{},
			},
		},
	}
	_ = tc.typeExpr(nil, expr)
	require.Equal(t, 1, len(tc.exprs))
	require.Equal(t, "Object key must be a string, got *ast.BooleanExpr",
		tc.exprs[expr].(*schema.InvalidType).Diagnostics[0].Summary)
}

func TestAliasesTypeChecking(t *testing.T) {
	t.Parallel()

	t.Run("valid string aliases", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-valid-string
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - "urn:pulumi:stack::project::test:resource:type::oldName"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "valid string aliases should not produce errors")
	})

	t.Run("valid object aliases", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-valid-object
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - name: oldName
        - type: test:resource:OldType
        - noParent: true
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "valid object aliases should not produce errors")
	})

	t.Run("mixed string and object aliases", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-mixed
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - "urn:pulumi:stack::project::test:resource:type::oldName"
        - name: anotherOldName
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "mixed string and object aliases should not produce errors")
	})

	t.Run("parent field with resource reference", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-parent
runtime: yaml
resources:
  parentRes:
    type: test:resource:type
    properties:
      foo: oof
  childRes:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - parent: ${parentRes}
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "parent field with resource reference should not produce errors")
	})

	t.Run("invalid field in alias object", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-invalid-field
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - invalidField: someValue
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "invalid field in alias object should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about invalidField not being a valid property
			var foundInvalidFieldError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "invalidField") {
					foundInvalidFieldError = true
					break
				}
			}
			assert.True(t, foundInvalidFieldError, "should report error about invalid field: %v", diagStrings)
		}
	})

	t.Run("wrong type for noParent", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-wrong-type
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - noParent: "true"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "wrong type for noParent should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about type mismatch
			var foundTypeError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "noParent") || strings.Contains(ds, "bool") || strings.Contains(ds, "string") {
					foundTypeError = true
					break
				}
			}
			assert.True(t, foundTypeError, "should report type error for noParent: %v", diagStrings)
		}
	})

	t.Run("wrong type for name", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-wrong-name-type
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - name: 123
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "wrong type for name should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about type mismatch
			var foundTypeError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "name") || strings.Contains(ds, "string") || strings.Contains(ds, "number") {
					foundTypeError = true
					break
				}
			}
			assert.True(t, foundTypeError, "should report type error for name: %v", diagStrings)
		}
	})

	t.Run("wrong type for parent", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-wrong-parent-type
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - parent: "someString"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Detail, "Cannot assign type 'string' to type 'pulumi:pulumi:Any'")
	})

	t.Run("non-array aliases", func(t *testing.T) {
		t.Parallel()
		text := `
name: test-aliases-not-array
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases: "urn:pulumi:stack::project::test:resource:type::oldName"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Equal(t, syntax.Diagnostics{{
			Diagnostic: hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary: "List<Union<string, {name: string, noParent: boolean, parent: " +
					"pulumi:pulumi:Any, parentUrn: string, project: string, stack: " +
					"string, type: string, urn: string}>> is not assignable from string",
				Detail: "Cannot assign type 'string' to type 'List<Union<string, {name: " +
					"string, noParent: boolean, parent: pulumi:pulumi:Any, parentUrn: " +
					"string, project: string, stack: string, type: string, urn: string}>>'",
				Subject: &hcl.Range{
					Filename: "<stdin>",
					Start:    hcl.Pos{Line: 9, Column: 16},
					End:      hcl.Pos{Line: 9, Column: 70},
				},
			},
		}}, diags)
	})
}
