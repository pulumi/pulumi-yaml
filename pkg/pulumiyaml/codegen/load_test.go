// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
)

func TestImportTemplate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// A Pulumi YAML program
		input string
		// A PCL program
		expected string

		diagErrors []string
	}{
		{
			name: "complex resource options",
			input: `
resources:
  prov:
    type: test:mod:prov
  bar:
    type: test:mod:typ
    options:
      provider: ${prov.outputField[0].outputProvider}
`,
			expected: `resource prov "test:mod:prov" {
	__logicalName = "prov"
}

resource bar "test:mod:typ" {
	__logicalName = "bar"

	options {
		provider = prov.outputField[0].outputProvider
	}
}
`,
		},
		{
			name: "func name shadowing",
			input: `
outputs:
  cwd: "${pulumi.cwd}"
  stack: "${pulumi.stack}"
  project: "${pulumi.project}"
`,
			expected: `output cwd0 {
	__logicalName = "cwd"
	value = cwd()
}

output stack0 {
	__logicalName = "stack"
	value = stack()
}

output project0 {
	__logicalName = "project"
	value = project()
}
`,
		},
		{
			name: "complex pulumi variables",
			input: `
resources:
  bar:
    type: test:mod:typ
    properties:
      foo: ${pulumi.cwd}
`,
			expected: `resource bar "test:mod:typ" {
	__logicalName = "bar"
	foo = cwd()
}
`,
		},
		{
			name: "invalid pulumi variable",
			input: `
outputs:
  foo: ${pulumi.bar}
`,
			diagErrors: []string{"invalid pulumi variable.yaml:3,8-21: " +
				"Unknown property of the `pulumi` variable: 'bar'; "},
		},
		{
			name: "interpolate pulumi variable",
			input: `
outputs:
  foo: ${pulumi.cwd}/folder
`,
			expected: `output foo {
	__logicalName = "foo"
	value = "${cwd()}/folder"
}
`,
		},
		{
			name: "nested-map-declaration",
			input: `
resources:
  my-bucket:
    type: aws:s3:Bucket
    properties:
      website:
        indexDocument: index.html
`,
			expected: `resource myBucket "aws:s3/bucket:Bucket" {
	__logicalName = "my-bucket"
	website = {
		indexDocument = "index.html"
	}
}
`,
		},
		{
			name: "invokes",
			input: `
variables:
  ret:
    Fn::Invoke:
      Function: test:mod:fn
      Return: foo
  noRet:
    Fn::Invoke:
      Function: test:mod:fn`,
			expected: `ret = invoke("test:mod:fn", {}).foo
noRet = invoke("test:mod:fn", {})
`,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			decl, diags, err := pulumiyaml.LoadYAML(tt.name+".yaml", strings.NewReader(tt.input))
			require.NoError(t, err)
			require.False(t, diags.HasErrors(), diags)
			assert.Empty(t, diags)

			result, diags := ImportTemplate(decl, testPackageLoader{t})
			if tt.diagErrors == nil {
				require.False(t, diags.HasErrors(), diags)
				assert.Equal(t, tt.expected, fmt.Sprintf("%v", result))
				assert.Empty(t, diags)
			} else {
				require.True(t, diags.HasErrors())
				var diagErrors []string
				for _, err := range diags {
					diagErrors = append(diagErrors, err.Error())
				}
				assert.Equal(t, tt.diagErrors, diagErrors)
			}
		})

	}
}
