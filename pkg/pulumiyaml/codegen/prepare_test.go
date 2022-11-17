// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

const example = `
name: simple-yaml
runtime: yaml
config:
  bucketNamePrefix: 3
resources:
  my-bucket:
    type: aws:s3/bucket:Bucket
    properties:
      bucketPrefix: ${bucketNamePrefix}
`

func TestMissingExternalConfig(t *testing.T) {
	t.Parallel()

	syntax, diags := encoding.DecodeYAML("<stdin>", yaml.NewDecoder(strings.NewReader(example)), nil)
	require.Len(t, diags, 0)

	template, diags := ast.ParseTemplate([]byte(example), syntax)
	assert.Len(t, diags, 0)

	assert.Nil(t, template.Description)

	host, err := newPluginHost()
	assert.Nil(t, err)
	loader := schema.NewPluginLoader(host)
	defer contract.IgnoreClose(host)

	pkgLoader := pulumiyaml.NewPackageLoaderFromSchemaLoader(loader)
	_, diags, err = pulumiyaml.PrepareTemplate(template, nil, pkgLoader)
	assert.Nil(t, err)
	assert.Equal(t, "<stdin>:10,21-40: resource, variable, or config value \"bucketNamePrefix\" not found; ", diags.Error())
}
