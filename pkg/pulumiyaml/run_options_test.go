// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

const fakeName = "foo"

type FakePackage struct {
	t *testing.T
}

func (m FakePackage) ResolveResource(typeName string) (ResourceTypeToken, error) {
	switch typeName {
	case fakeName:
		return ResourceTypeToken(typeName), nil
	default:
		assert.Fail(m.t, "Unexpected type token %q", typeName)
		return "", fmt.Errorf("Unexpected type token %q", typeName)
	}
}

func (m FakePackage) IsComponent(typeName ResourceTypeToken) (bool, error) {
	switch typeName.String() {
	case fakeName:
		return false, nil
	default:
		assert.Fail(m.t, "Unexpected type token %q", typeName)
		return false, fmt.Errorf("Unexpected type token %q", typeName)
	}
}

func (m FakePackage) ResourceTypeHint(typeName ResourceTypeToken) *schema.ResourceType {
	switch typeName.String() {
	case fakeName:
		return nil
	default:
		assert.Fail(m.t, "Unexpected type token %q", typeName)
		return nil

	}
}

func (m FakePackage) ResourceConstants(typeName ResourceTypeToken) map[string]interface{} {
	return nil
}

func TestResourceOptions(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  provider-a:
    type: pulumi:providers:test
  provider-b:
    type: pulumi:providers:test
  res-parent:
    type: test:resource:trivial
  res-dependency:
    type: test:resource:trivial
  res-a:
    type: test:component:type
    options:
      protect: true
      provider: ${provider-a}
      providers:
      - ${provider-a}
      parent: ${res-parent}
      dependsOn:
      - ${res-dependency}
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "pulumi:providers:test":
				return "providerId", resource.PropertyMap{}, nil
			case "test:resource:trivial":
				return "resourceId", resource.PropertyMap{}, nil
			case testComponentToken:
				assert.Equal(t, "urn:pulumi:stackDev::projectFoo::pulumi:providers:test::provider-a::providerId", args.RegisterRPC.Provider)
				assert.Equal(t, map[string]string{
					"test": "urn:pulumi:stackDev::projectFoo::pulumi:providers:test::provider-a::providerId",
				}, args.RegisterRPC.GetProviders())
				assert.Equal(t, "urn:pulumi:stackDev::projectFoo::test:resource:trivial::res-parent", args.RegisterRPC.Parent)
				assert.Contains(t, args.RegisterRPC.Dependencies,
					"urn:pulumi:stackDev::projectFoo::test:resource:trivial::res-dependency",
				)

				return "anID", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template, newMockPackageMap())
		diags := runner.Evaluate()
		requireNoErrors(t, template, diags)
		return nil
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	assert.NoError(t, err)
}

func TestDefaultProvider(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  provider-a:
    type: pulumi:providers:test
    defaultProvider: true
  res-a:
    type: test:component:type
variables:
  var-a:
    Fn::Invoke:
      Function: test:invoke:type
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "pulumi:providers:test":
				return "providerId", resource.PropertyMap{}, nil
			case testComponentToken:
				assert.Equal(t, "urn:pulumi:stackDev::projectFoo::pulumi:providers:test::provider-a::providerId", args.RegisterRPC.Provider)
				return "anID", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
		CallF: func(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
			t.Logf("Processing call %s.", args.Token)
			switch args.Token {
			case "test:invoke:type":
				assert.Equal(t, args.Provider, "urn:pulumi:stackDev::projectFoo::pulumi:providers:test::provider-a::providerId")
				return resource.PropertyMap{
					"retval": resource.NewStringProperty("oof"),
				}, nil
			}
			return resource.PropertyMap{}, fmt.Errorf("Unexpected invoke %s", args.Token)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template, newMockPackageMap())
		assert.Equal(t, runner.setDefaultProviders(), nil)
		diags := runner.Evaluate()
		requireNoErrors(t, template, diags)
		return nil
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	assert.NoError(t, err)
}
