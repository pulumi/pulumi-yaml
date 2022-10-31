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

const (
	testInvokeFnToken  = "test:invoke:type"
	testProvidersToken = "pulumi:providers:test"
	providerIDAttr     = "providerId"
)

func TestInvokeOutputs(t *testing.T) {
	t.Parallel()

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
        fn::invoke:
          function: test:invoke:type
          arguments:
            quux: ${res-a.out}
          return: retval
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeWithOptsOutputs(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  provider-a:
    type: pulumi:providers:test
  res-b:
    type: test:resource:type
    properties:
      foo:
        fn::invoke:
          function: test:invoke:type2
          arguments:
            quux: ${res-a.out}
          options:
            Provider: ${provider-a}
          return: retval
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeVariable(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
variables:
  foo:
    fn::invoke:
      function: test:invoke:type
      arguments:
        quux: tuo
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: ${foo.retval}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeOutputVariable(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
variables:
  foo:
    fn::invoke:
      function: test:invoke:type
      arguments:
        quux: ${res-a.out}
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-b:
    type: test:resource:type
    properties:
      foo: ${foo.retval}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeNoInputs(t *testing.T) {
	t.Parallel()

	const text = `
variables:
  config:
    fn::invoke:
      function: test:invoke:empty
outputs:
  v: ${config.subscriptionId}
name: test-yaml
runtime: yaml
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeVariableSugar(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
variables:
  foo:
    fn::test:invoke:type:
      quux: tuo
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: ${foo.retval}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeOutputVariableSugar(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
variables:
  foo:
    fn::test:invoke:type:
      quux: ${res-a.out}
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-b:
    type: test:resource:type
    properties:
      foo: ${foo.retval}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestInvokeNoInputsSugar(t *testing.T) {
	t.Parallel()

	const text = `
variables:
  config:
    fn::test:invoke:empty: {}
outputs:
  v: ${config.subscriptionId}
name: test-yaml
runtime: yaml
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func testInvokeDiags(t *testing.T, template *ast.TemplateDecl, callback func(*Runner)) syntax.Diagnostics {
	mocks := &testMonitor{
		CallF: func(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
			t.Logf("Processing call %s.", args.Token)
			switch args.Token {
			case testInvokeFnToken:
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"quux": "tuo",
				}), args.Args)
				return resource.PropertyMap{
					"retval": resource.NewStringProperty("oof"),
				}, nil
			case "test:invoke:type2":
				assert.Equal(t, args.Provider, "urn:pulumi:dev::foo::pulumi:providers:test::provider-a::providerId")
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"quux": "tuo",
				}), args.Args)
				return resource.PropertyMap{
					"retval": resource.NewStringProperty("oof"),
				}, nil
			case "test:invoke:empty":
				return nil, nil
			case "test:invoke:poison":
				return nil, fmt.Errorf("Don't eat the poison")
			}
			return resource.PropertyMap{}, fmt.Errorf("Unexpected invoke %s", args.Token)
		},
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			if args.ReadRPC != nil {
				switch args.TypeToken {
				case "test:read:Resource":
					if args.ID != "no-state" {
						assert.Equal(t, "bucket-123456", args.ID)
						assert.Equal(t, `string_value:"bar"`, args.ReadRPC.Properties.Fields["foo"].String())
						assert.Len(t, args.ReadRPC.Properties.Fields, 1)
					}
					return "arn:aws:s3:::" + args.ID, resource.PropertyMap{
						"tags": resource.NewObjectProperty(resource.PropertyMap{
							"isRight": resource.NewStringProperty("yes"),
						}),
					}, nil
				}
				return "", resource.PropertyMap{}, fmt.Errorf("Unexpected read resource type %s", args.TypeToken)
			}
			switch args.TypeToken {
			case testProvidersToken:
				return providerIDAttr, resource.PropertyMap{
					"retval": resource.NewStringProperty("provider-foo"),
				}, nil
			case testResourceToken:
				assert.Equal(t, testResourceToken, args.TypeToken)
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				}), args.Inputs, "expected resource test:resource:type to have property foo: oof")
				assert.Equal(t, "", args.Provider)
				assert.Equal(t, "", args.ID)

				return "not-tested-here", resource.PropertyMap{
					"foo":    resource.NewStringProperty("qux"),
					"bar":    resource.NewStringProperty("oof"),
					"out":    resource.NewStringProperty("tuo"),
					"outNum": resource.NewNumberProperty(1),
				}, nil
			case testComponentToken:
				assert.Equal(t, testComponentToken, args.TypeToken)
				assert.True(t, args.Inputs.DeepEquals(resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				})))
				assert.Equal(t, "", args.Provider)
				assert.Equal(t, "", args.ID)

				return "", resource.PropertyMap{}, nil
			case "test:resource:not-run":
				assert.Fail(t, "The 'not-run' resource was constructed")
				return "not-run", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(template, newMockPackageMap())
		err := runner.Evaluate(ctx, nil)
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
