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
}

resource bar "test:mod:typ" {
	options {
		provider = prov.outputField[0].outputProvider
	}
}
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

			result, diags := ImportTemplate(decl)
			require.False(t, diags.HasErrors(), diags)
			assert.Equal(t, tt.expected, fmt.Sprintf("%v", result))
			assert.Empty(t, diags)
		})

	}
}
