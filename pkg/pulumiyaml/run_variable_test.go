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

// Test that a variable can be prior to any resource in the topological sort:
//
//  1. Resource `res-a` depends on variable `someVar`
//  2. Variable `someVar` has no dependencies
func TestVariableInput(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
variables:
  someVar:
    Fn::Invoke:
      Function: test:invoke:type
      Arguments:
        quux: tuo
      Return: retval
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: ${someVar}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testVariableDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

// Test that a variable can be between two resources in the topological sort:
//
//  1. Resource `res-b` depends on variable `someVar`
//  2. Variable `someVar` depends on resource `res-a`
//  3. `res-a` has no dependencies
func TestVariableIntermediate(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
variables:
  someVar:
    Fn::Invoke:
      Function: test:invoke:type
      Arguments:
        quux: ${res-a.out}
      Return: retval
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-b:
    type: test:resource:type
    properties:
      foo: ${someVar}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testVariableDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

// Test that a variable can be between two resources in the topological sort:
//
//  1. Resource `res-b` depends on variable `someVar`
//  2. Variable `someVar` depends on variable `passthrough`
//  2. Variable `passthrough` depends on resource `res-a`
//  3. `res-a` has no dependencies
func TestVariableDoubleIntermediate(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
variables:
  passthrough:
    Fn::Invoke:
      Function: test:invoke-passthrough:type
      Arguments:
        returnValue: ${res-a.out}
      Return: returnValue
  someVar:
    Fn::Invoke:
      Function: test:invoke:type
      Arguments:
        quux: ${passthrough}
      Return: retval
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-b:
    type: test:resource:type
    properties:
      foo: ${someVar}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testVariableDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

// Test that a variable with can be after every resource in the topological sort:
//
//  1. Variable `someVar` depends on resource `res-a`
//  1. Resource `res-a` depends on nothing
func TestVariableOutput(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
variables:
  someVar:
    Fn::Invoke:
      Function: test:invoke:type
      Arguments:
        quux: ${res-a.out}
      Return: retval
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
outputs:
  out: ${someVar}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testVariableDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

func TestVariableMemozied(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
variables:
  someVar:
    Fn::Invoke:
      Function: test:invoke:type
      Arguments:
        quux: ${res-a.out}
      Return: retval
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-b:
    type: test:resource:type
    properties:
      foo: ${someVar}
  res-c:
    type: test:resource:type
    properties:
      foo: ${someVar}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testVariableDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

func testVariableDiags(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) syntax.Diagnostics {
	testInvokeCalls := 0

	mocks := &testMonitor{
		CallF: func(token string, args resource.PropertyMap, provider string) (resource.PropertyMap, error) {
			switch token {
			case "test:invoke-passthrough:type":
				// returns the same shape as the arguments
				return args, nil
			case "test:invoke:type":
				testInvokeCalls++
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
			case testResourceToken:
				assert.Equal(t, testResourceToken, typeToken)
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				}), state, "expected resource test:resource:type to have property foo: oof")
				assert.Equal(t, "", provider)
				assert.Equal(t, "", id)

				return "not-tested-here", resource.PropertyMap{
					"foo":    resource.NewStringProperty("qux"),
					"bar":    resource.NewStringProperty("oof"),
					"out":    resource.NewStringProperty("tuo"),
					"outNum": resource.NewNumberProperty(1),
				}, nil
			case testComponentToken:
				assert.Equal(t, testComponentToken, typeToken)
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
	assert.Equal(t, 1, testInvokeCalls)
	return nil
}
