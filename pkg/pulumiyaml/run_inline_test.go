// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	_ "embed"
	"fmt"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunInline(t *testing.T) {
	t.Parallel()
	text := `
name: consumer
runtime: yaml
inputs:
  foo:
    type: string
resources:
  bucket:
    type: test:resource:type
    properties:
      bar: ${foo}
outputs:
  baz: ${foo}
`
	inputValue := "this is a test"

	template := yamlTemplate(t, text)
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case testResourceToken:
				assert.Equal(t, args.Inputs["bar"], resource.NewStringProperty(inputValue))
				return "someID", resource.PropertyMap{
					"bar": resource.NewStringProperty("qux"),
				}, nil
			case testComponentToken:
				return "", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}
	runner := NewRunner(template, newMockPackageMap())
	_, diags := TypeCheck(runner, nil)
	require.NoError(t, diags)

	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		outputs, err := RunInlineTemplate(ctx, template, map[string]interface{}{
			"foo": "this is a test",
		}, newMockPackageMap())
		require.NoError(t, err)

		outputs["baz"].(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "tuo", x)
			return nil, nil
		})

		return nil
	}, pulumi.WithMocks("test", "gen", mocks))
	require.NoError(t, err)
}
