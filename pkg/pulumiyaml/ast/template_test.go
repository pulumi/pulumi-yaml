// Copyright 2022, Pulumi Corporation.  All rights reserved.

package ast

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
)

const example = `
name: simple-yaml
runtime: yaml
config:
  some-string-array:
    type: array
    value:
      - subnet1
      - subnet2
      - subnet3
    items:
      type: string
  some-nested-array:
    type: array
    items:
      type: array
      items:
        type: string
  some-boolean:
    type: boolean
resources:
  my-bucket:
    type: aws:s3/bucket:Bucket
    properties:
      website:
        indexDocument: index.html
  index.html:
    type: aws:s3/bucketObject:BucketObject
    properties:
      bucket: ${my-bucket}
      source:
        fn::stringAsset: <h1>Hello, world!</h1>
      acl: public-read
      contentType: text/html
outputs:
  # Export the bucket's endpoint
  bucketEndpoint: http://${my-bucket.websiteEndpoint}
`

func TestExample(t *testing.T) {
	t.Parallel()

	syntax, diags := encoding.DecodeYAML("<stdin>", yaml.NewDecoder(strings.NewReader(example)), nil)
	require.Len(t, diags, 0)

	template, diags := ParseTemplate([]byte(example), syntax)
	assert.Len(t, diags, 0)

	assert.Nil(t, template.Description)
}
