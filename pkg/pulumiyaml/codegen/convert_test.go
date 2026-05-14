// Copyright 2026, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"context"
	"fmt"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
)

// snippetPackage is a minimal Package implementation used to exercise ImportSnippet.
//
// It defines:
//   - resource "snippet:index:Bucket" with input properties of mixed types,
//   - function "snippet:index:lookup" with an input object,
//   - the provider for "snippet" via the "pulumi:providers:snippet" token.
type snippetPackage struct{}

func (snippetPackage) Name() string             { return "snippet" }
func (snippetPackage) Version() *semver.Version { return nil }

func (snippetPackage) ResolveResource(typeName string) (pulumiyaml.ResourceTypeToken, error) {
	switch typeName {
	case "snippet:index:Bucket", "pulumi:providers:snippet":
		return pulumiyaml.ResourceTypeToken(typeName), nil
	}
	return "", fmt.Errorf("snippetPackage: unexpected resource %q", typeName)
}

func (snippetPackage) ResolveFunction(typeName string) (pulumiyaml.FunctionTypeToken, error) {
	if typeName == "snippet:index:lookup" {
		return pulumiyaml.FunctionTypeToken(typeName), nil
	}
	return "", fmt.Errorf("snippetPackage: unexpected function %q", typeName)
}

func (snippetPackage) IsComponent(pulumiyaml.ResourceTypeToken) (bool, error) { return false, nil }

func (snippetPackage) IsResourcePropertySecret(pulumiyaml.ResourceTypeToken, string) (bool, error) {
	return false, nil
}

func (snippetPackage) ResourceTypeHint(typeName pulumiyaml.ResourceTypeToken) *schema.ResourceType {
	tagsMap := &schema.MapType{ElementType: schema.StringType}
	switch typeName {
	case "snippet:index:Bucket":
		return &schema.ResourceType{
			Token: string(typeName),
			Resource: &schema.Resource{
				Token: string(typeName),
				InputProperties: []*schema.Property{
					{Name: "name", Type: schema.StringType},
					{Name: "size", Type: schema.IntType},
					{Name: "versioned", Type: schema.BoolType},
					{Name: "tags", Type: tagsMap},
				},
			},
		}
	case "pulumi:providers:snippet":
		return &schema.ResourceType{
			Token: string(typeName),
			Resource: &schema.Resource{
				Token: string(typeName),
				InputProperties: []*schema.Property{
					{Name: "region", Type: schema.StringType},
					{Name: "insecure", Type: schema.BoolType},
				},
			},
		}
	}
	return nil
}

func (snippetPackage) FunctionTypeHint(typeName pulumiyaml.FunctionTypeToken) *schema.Function {
	if typeName != "snippet:index:lookup" {
		return nil
	}
	return &schema.Function{
		Token: string(typeName),
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "id", Type: schema.StringType},
				{Name: "limit", Type: schema.IntType},
			},
		},
	}
}

func (snippetPackage) ResourceConstants(pulumiyaml.ResourceTypeToken) map[string]interface{} {
	return nil
}

type snippetPackageLoader struct{}

func (snippetPackageLoader) LoadPackage(
	_ context.Context, descriptor *schema.PackageDescriptor,
) (pulumiyaml.Package, error) {
	if descriptor.Name != "snippet" {
		return nil, fmt.Errorf("snippetPackageLoader: unexpected package %q", descriptor.Name)
	}
	return snippetPackage{}, nil
}

func (snippetPackageLoader) Close() {}

func TestImportSnippet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		token    string
		filename string
		input    string
		expected string
	}{
		{
			name:     "resource inputs",
			token:    "snippet:index:Bucket",
			filename: "inputs.yaml",
			input: `name: my-bucket
size: 42
versioned: true
tags:
  env: prod
  team: platform
`,
			expected: `name = "my-bucket"
size = 42
versioned = true
tags = {
	"env" = "prod",
	"team" = "platform"
}
`,
		},
		{
			name:     "function inputs",
			token:    "snippet:index:lookup",
			filename: "args.yaml",
			input: `id: abc-123
limit: 10
`,
			expected: `id = "abc-123"
limit = 10
`,
		},
		{
			name:     "provider inputs",
			token:    "pulumi:providers:snippet",
			filename: "provider.yaml",
			input: `region: us-west-2
insecure: false
`,
			expected: `region = "us-west-2"
insecure = false
`,
		},
		{
			name:     "unknown property has no hint",
			token:    "snippet:index:Bucket",
			filename: "inputs.yaml",
			// "extra" isn't in the schema; the importer still emits it but treats the value as
			// an interpolated string (quoted) rather than a plain literal.
			input: `name: my-bucket
extra: hello
`,
			expected: `name = "my-bucket"
extra = "hello"
`,
		},
		{
			name:     "fn::length builtin",
			token:    "snippet:index:Bucket",
			filename: "inputs.yaml",
			input: `name: my-bucket
size:
  fn::length:
    - a
    - b
    - c
`,
			expected: `name = "my-bucket"
size = length([
	"a",
	"b",
	"c"
])
`,
		},
		{
			name:     "fn::join + pulumi.cwd",
			token:    "snippet:index:Bucket",
			filename: "inputs.yaml",
			input: `name:
  fn::join:
    - "-"
    - - prefix
      - ${pulumi.cwd}
`,
			expected: `name = join("-", [
	"prefix",
	cwd()
])
`,
		},
		{
			name:     "fn::toJSON of nested structure",
			token:    "snippet:index:Bucket",
			filename: "inputs.yaml",
			input: `name:
  fn::toJSON:
    foo: bar
    nested:
      - 1
      - 2
`,
			expected: `name = toJSON({
	"foo" = "bar",
	"nested" = [
		1,
		2
	]
})
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body, diags, err := ImportSnippet(
				t.Context(), tt.token, tt.filename, []byte(tt.input), snippetPackageLoader{})
			require.NoError(t, err)
			require.Falsef(t, diags.HasErrors(), "unexpected diagnostics: %v", diags)
			assert.Equal(t, tt.expected, fmt.Sprintf("%v", body))
		})
	}
}

func TestImportSnippet_UnknownToken(t *testing.T) {
	t.Parallel()
	_, _, err := ImportSnippet(
		t.Context(), "snippet:index:DoesNotExist", "inputs.yaml",
		[]byte("name: foo\n"), snippetPackageLoader{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a resource, function, or provider")
}

func TestImportSnippet_NotAMapping(t *testing.T) {
	t.Parallel()
	_, diags, err := ImportSnippet(
		t.Context(), "snippet:index:Bucket", "inputs.yaml",
		[]byte("- 1\n- 2\n"), snippetPackageLoader{})
	require.NoError(t, err)
	require.True(t, diags.HasErrors(), "expected diagnostics for non-mapping input")
	assert.Contains(t, diags.Error(), "must be an object")
}
