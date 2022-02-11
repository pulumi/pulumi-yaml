// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

func TestInvokeOutputs(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-b:
    type: test:resource:type
    properties:
      foo:
        Fn::Invoke:
          Function: test:invoke:type
          Arguments:
            quux: ${res-a.out}
          Return: retval
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

func testInvokeDiags(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) syntax.Diagnostics {
	mocks := &testMonitor{
		CallF: func(token string, args resource.PropertyMap, provider string) (resource.PropertyMap, error) {
			switch token {
			case "test:invoke:type":
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"quux": "tuo",
				}), args)
				return resource.PropertyMap{
					"retval": resource.NewStringProperty("oof"),
				}, nil
			}
			return resource.PropertyMap{}, fmt.Errorf("Unexpected invoke %s", token)
		},
		NewResourceF: func(typeToken, name string, state resource.PropertyMap,
			provider, id string) (string, resource.PropertyMap, error) {

			switch typeToken {
			case "test:resource:type":
				assert.Equal(t, "test:resource:type", typeToken)
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				}), state, "expected resource test:resource:type to have property foo: oof")
				assert.Equal(t, "", provider)
				assert.Equal(t, "", id)

				return "someID", resource.PropertyMap{
					"foo":    resource.NewStringProperty("qux"),
					"bar":    resource.NewStringProperty("oof"),
					"out":    resource.NewStringProperty("tuo"),
					"outNum": resource.NewNumberProperty(1),
				}, nil
			case "test:component:type":
				assert.Equal(t, "test:component:type", typeToken)
				assert.True(t, state.DeepEquals(resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				})))
				assert.Equal(t, "", provider)
				assert.Equal(t, "", id)

				return "", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", typeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template)
		err := runner.Evaluate()
		if err != nil {
			return err
		}
		if callback != nil {
			callback(runner)
		}
		return nil
	}, pulumi.WithMocks("foo", "dev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		return diags
	}
	assert.NoError(t, err)
	return nil
}
