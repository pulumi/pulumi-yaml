// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

func TestResourceOptions(t *testing.T) {
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
    type: test:resource:type
    component: true # required to set "remote: true" in engine and parse providers option
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
			case testResourceToken:
				assert.Equal(t, "urn:pulumi:stackDev::projectFoo::pulumi:providers:test::provider-a::providerId", args.RegisterRPC.Provider)
				assert.Equal(t, map[string]string{
					"test": "urn:pulumi:stackDev::projectFoo::pulumi:providers:test::provider-a::providerId",
				}, args.RegisterRPC.GetProviders())
				assert.Equal(t, "urn:pulumi:stackDev::projectFoo::test:resource:trivial::res-parent", args.RegisterRPC.Parent)
				assert.Equal(t, []string{
					"urn:pulumi:stackDev::projectFoo::test:resource:trivial::res-dependency",
				}, args.RegisterRPC.Dependencies)

				return "anID", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template)
		diags := runner.Evaluate()
		requireNoErrors(t, template, diags)
		return nil
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	assert.NoError(t, err)
}
