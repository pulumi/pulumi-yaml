// Copyright 2022, Pulumi Corporation.  All rights reserved.

package ast

import (
	"encoding/json"
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

const componentExample = `
name: yaml-plugin
runtime: yaml
components:
  aComponent:
    config:
      someStringArray:
        type: array
        items:
          type: string
    resources:
      myBucket:
        type: aws:s3/bucket:Bucket
        properties:
          acl: private
    outputs:
      bucketEndpoint: http://${myBucket.websiteEndpoint}
  anotherComponent:
    resources:
      differentBucket:
        type: aws:s3/bucket:Bucket
        properties:
          acl: public-read
    outputs:
      bucketEndpoint: http://${differentBucket.websiteEndpoint}
`

func TestComponentParsing(t *testing.T) {
	t.Parallel()

	syntax, diags := encoding.DecodeYAML("<stdin>", yaml.NewDecoder(strings.NewReader(componentExample)), nil)
	require.Len(t, diags, 0)

	template, diags := ParseTemplate([]byte(componentExample), syntax)
	require.Len(t, diags, 0)

	require.Len(t, template.Components.Entries, 2)
	require.Equal(t, "aComponent", template.Components.Entries[0].Value.Name.Value)
	require.Len(t, template.Components.Entries[0].Value.Config.Entries, 1)
	require.Equal(t, "someStringArray", template.Components.Entries[0].Value.Config.Entries[0].Key.Value)
	require.Equal(t, "array", template.Components.Entries[0].Value.Config.Entries[0].Value.Type.Value)
	require.Equal(t, "string", template.Components.Entries[0].Value.Config.Entries[0].Value.Items.Type.Value)
	require.Len(t, template.Components.Entries[0].Value.Resources.Entries, 1)
	require.Equal(t, "myBucket", template.Components.Entries[0].Value.Resources.Entries[0].Key.Value)
	require.Len(t, template.Components.Entries[0].Value.Outputs.Entries, 1)
	require.Equal(t, "bucketEndpoint", template.Components.Entries[0].Value.Outputs.Entries[0].Key.Value)

	require.Equal(t, "anotherComponent", template.Components.Entries[1].Value.Name.Value)
	require.Nil(t, template.Components.Entries[1].Value.Config.Entries)
	require.Len(t, template.Components.Entries[1].Value.Resources.Entries, 1)
	require.Equal(t, "differentBucket", template.Components.Entries[1].Value.Resources.Entries[0].Key.Value)
	require.Len(t, template.Components.Entries[1].Value.Outputs.Entries, 1)
	require.Equal(t, "bucketEndpoint", template.Components.Entries[1].Value.Outputs.Entries[0].Key.Value)
}

const componentSchemaExample = `
name: yaml-plugin
description: A YAML plugin
runtime: yaml
components:
  aComponent:
    description: "A component"
    config:
      someStringArray:
        type: array
        items:
          type: string
      aString:
        type: string
      aNumber:
        type: integer
      aBoolean:
        type: boolean
      aStringWithDefault:
        type: string
        default: "default"
      aNumberWithDefault:
        type: integer
        default: 42
      aBooleanWithDefault:
        type: boolean
        default: true
    resources:
      myBucket:
        type: aws:s3/bucket:Bucket
    outputs:
      someOutput: "abcd"
`

func TestComponentSchemaGeneration(t *testing.T) {
	t.Parallel()

	syntax, diags := encoding.DecodeYAML("<stdin>", yaml.NewDecoder(strings.NewReader(componentSchemaExample)), nil)
	require.Len(t, diags, 0)

	template, diags := ParseTemplate([]byte(componentSchemaExample), syntax)
	require.Len(t, diags, 0)

	spec, err := template.GenerateSchema()
	require.NoError(t, err)

	marshalled, err := json.Marshal(spec)
	require.NoError(t, err)

	expectedSchema := `
{
  "name": "yaml-plugin",
  "description": "A YAML plugin",
  "language": {
    "cshap": {
      "respectSchemaVersion": true
    },
    "go": {
      "respectSchemaVersion": true
    },
    "java": {
      "respectSchemaVersion": true
    },
    "nodejs": {
      "respectSchemaVersion": true
    },
    "python": {
      "respectSchemaVersion": true
    }
  },
  "config": {},
  "provider": {},
  "resources": {
    "yaml-plugin:index:aComponent": {
      "description": "A component",
      "properties": {
        "someOutput": {
          "$ref": "pulumi.json#/Any"
        }
      },
      "type": "yaml-plugin:index:aComponent",
      "required": [
        "someOutput"
      ],
      "inputProperties": {
        "aBoolean": {
          "type": "boolean",
          "defaultInfo": {
            "environment": [
              "aBoolean"
            ]
          }
        },
        "aBooleanWithDefault": {
          "type": "boolean",
          "default": true,
          "defaultInfo": {
            "environment": [
              "aBooleanWithDefault"
            ]
          }
        },
        "aNumber": {
          "type": "integer",
          "defaultInfo": {
            "environment": [
              "aNumber"
            ]
          }
        },
        "aNumberWithDefault": {
          "type": "integer",
          "default": 42,
          "defaultInfo": {
            "environment": [
              "aNumberWithDefault"
            ]
          }
        },
        "aString": {
          "type": "string",
          "defaultInfo": {
            "environment": [
              "aString"
            ]
          }
        },
        "aStringWithDefault": {
          "type": "string",
          "default": "default",
          "defaultInfo": {
            "environment": [
              "aStringWithDefault"
            ]
          }
        },
        "someStringArray": {
          "type": "array",
          "items": {
            "type": "string"
          },
          "defaultInfo": {
            "environment": [
              "someStringArray"
            ]
          }
        }
      },
      "requiredInputs": [
        "someStringArray",
        "aString",
        "aNumber",
        "aBoolean"
      ],
      "isComponent": true
    }
  }
}
`
	require.JSONEq(t, expectedSchema, string(marshalled))
}
