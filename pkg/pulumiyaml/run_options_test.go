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
	"github.com/stretchr/testify/require"
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
configuration:
  shouldProtect:
    default: false
    type: boolean
resources:
  provider-a:
    type: pulumi:providers:test
  provider-b:
    type: pulumi:providers:test
  res-parent:
    type: test:resource:trivial
  res-dependency:
    type: test:resource:trivial
  res-container:
    type: test:resource:trivial
    options:
      protect: ${shouldProtect}
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
  res-b:
    type: test:resource:trivial
    options:
      deletedWith: ${res-container}
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
		runner := newRunner(template, newMockPackageMap())
		diags := runner.Evaluate(ctx)
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
    fn::Invoke:
      function: test:invoke:type
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
		runner := newRunner(template, newMockPackageMap())
		runner.setDefaultProviders()
		requireNoErrors(t, template, runner.sdiags.diags)
		diags := runner.Evaluate(ctx)
		requireNoErrors(t, template, diags)
		return nil
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	assert.NoError(t, err)
}

func TestComponentResourceParent(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      inner:
        type: ` + testResourceToken + `
        properties:
          foo: bar
    outputs:
      out: ${inner.bar}
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	resourceCreated := false
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "pulumi:providers:test":
				return "providerId", resource.PropertyMap{}, nil
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case testResourceToken:
				resourceCreated = true
				assert.Equal(t,
					"urn:pulumi:stackDev::projectFoo::pulumi:providers:test::myProvider::providerId",
					args.Provider,
				)
				assert.Equal(t,
					"urn:pulumi:stackDev::projectFoo::test:index:myComponent::myComp",
					args.RegisterRPC.Parent,
				)
				return "innerID", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		var provider pulumi.ProviderResourceState
		err := ctx.RegisterResource("pulumi:providers:test", "myProvider", nil, &provider)
		if err != nil {
			return err
		}

		_, _, err = RunComponentTemplate(ctx,
			"test:index:myComponent", "myComp",
			pulumi.Providers(&provider),
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)
	assert.True(t, resourceCreated, "expected inner resource to be created")
}

func TestComponentInvokeParent(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    variables:
      invokeResult:
        fn::invoke:
          function: test:invoke:type
    outputs:
      out: ${invokeResult}
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	invokeCalled := false
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "pulumi:providers:test":
				return "providerId", resource.PropertyMap{}, nil
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
		CallF: func(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
			switch args.Token {
			case "test:invoke:type":
				invokeCalled = true
				assert.Equal(t,
					"urn:pulumi:stackDev::projectFoo::pulumi:providers:test::myProvider::providerId",
					args.Provider,
				)
				return resource.PropertyMap{
					"retval": resource.NewStringProperty("oof"),
				}, nil
			}
			return resource.PropertyMap{}, fmt.Errorf("unexpected invoke %s", args.Token)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		var provider pulumi.ProviderResourceState
		err := ctx.RegisterResource("pulumi:providers:test", "myProvider", nil, &provider)
		if err != nil {
			return err
		}

		_, _, err = RunComponentTemplate(ctx,
			"test:index:myComponent", "myComp",
			pulumi.Providers(&provider),
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)
	assert.True(t, invokeCalled, "expected invoke to be called")
}

// TestComponentResourceMultipleInstances exercises the fix for #957: instantiating the
// same YAML component twice in a single stack must not produce duplicate URNs. Each
// child resource's name is prefixed with the component instance name, and an alias to
// the un-prefixed name is emitted so existing single-instance stacks migrate cleanly.
func TestComponentResourceMultipleInstances(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      inner:
        type: ` + testResourceToken + `
        properties:
          foo: bar
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	var innerNames []string
	var innerAliasURNs [][]string
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case testResourceToken:
				innerNames = append(innerNames, args.Name)
				var aliasURNs []string
				if args.RegisterRPC != nil {
					for _, a := range args.RegisterRPC.Aliases {
						if u := a.GetUrn(); u != "" {
							aliasURNs = append(aliasURNs, u)
						}
					}
				}
				innerAliasURNs = append(innerAliasURNs, aliasURNs)
				return "innerID", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, _, err := RunComponentTemplate(ctx,
			"test:index:myComponent", "cp1", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		if err != nil {
			return err
		}
		_, _, err = RunComponentTemplate(ctx,
			"test:index:myComponent", "cp2", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"cp1-inner", "cp2-inner"}, innerNames)
	require.Len(t, innerAliasURNs, 2)
	// Both instances alias to the same un-prefixed URN — that was the singleton URN
	// before the fix. Each registration gets exactly one alias entry, scoped via
	// component-derived URN to its own component instance.
	expected := "urn:pulumi:stackDev::projectFoo::test:index:myComponent$" + testResourceToken + "::inner"
	for _, urns := range innerAliasURNs {
		assert.Equal(t, []string{expected}, urns)
	}
}

// TestComponentResourceNestedComposition verifies that prefixing composes correctly when
// a component is itself nested inside another component. The engine constructs a nested
// component by passing the already-prefixed name (e.g. "outer-inner") as `name` to
// RunComponentTemplate; children inside it should pick up the cumulative prefix
// "outer-inner-leaf".
func TestComponentResourceNestedComposition(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      leaf:
        type: ` + testResourceToken + `
        properties:
          foo: bar
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	var leafName string
	var leafAliasURNs []string
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case testResourceToken:
				leafName = args.Name
				if args.RegisterRPC != nil {
					for _, a := range args.RegisterRPC.Aliases {
						if u := a.GetUrn(); u != "" {
							leafAliasURNs = append(leafAliasURNs, u)
						}
					}
				}
				return "leafID", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		// Simulate the engine's construct of a nested component: it passes the
		// already-prefixed instance name "outer-inner" as the component's name.
		_, _, err := RunComponentTemplate(ctx,
			"test:index:myComponent", "outer-inner", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)
	assert.Equal(t, "outer-inner-leaf", leafName)
	// The alias URN is derived from the parent component's URN (whose name is
	// "outer-inner"), confirming the alias is scoped under the nested component.
	assert.Equal(t,
		[]string{"urn:pulumi:stackDev::projectFoo::test:index:myComponent$" + testResourceToken + "::leaf"},
		leafAliasURNs,
	)
}

// TestComponentResourceExplicitName verifies that an explicit `name:` field on a
// resource inside a component is also prefixed (and aliased to its un-prefixed form).
func TestComponentResourceExplicitName(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      yamlKey:
        type: ` + testResourceToken + `
        name: customName
        properties:
          foo: bar
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	var registeredName string
	var aliasURNs []string
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case testResourceToken:
				registeredName = args.Name
				if args.RegisterRPC != nil {
					for _, a := range args.RegisterRPC.Aliases {
						if u := a.GetUrn(); u != "" {
							aliasURNs = append(aliasURNs, u)
						}
					}
				}
				return "id", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, _, err := RunComponentTemplate(ctx,
			"test:index:myComponent", "cp1", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)
	assert.Equal(t, "cp1-customName", registeredName)
	assert.Equal(t,
		[]string{"urn:pulumi:stackDev::projectFoo::test:index:myComponent$" + testResourceToken + "::customName"},
		aliasURNs,
	)
}

// TestComponentResourceUserAliasesCombined verifies that a user-supplied alias on a
// resource inside a component is preserved alongside the auto-generated alias to the
// un-prefixed name.
func TestComponentResourceUserAliasesCombined(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      inner:
        type: ` + testResourceToken + `
        properties:
          foo: bar
        options:
          aliases:
            - name: legacyName
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	var aliasNames []string
	var aliasURNs []string
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case testResourceToken:
				if args.RegisterRPC != nil {
					for _, a := range args.RegisterRPC.Aliases {
						if u := a.GetUrn(); u != "" {
							aliasURNs = append(aliasURNs, u)
						}
						if spec := a.GetSpec(); spec != nil && spec.GetName() != "" {
							aliasNames = append(aliasNames, spec.GetName())
						}
					}
				}
				return "id", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, _, err := RunComponentTemplate(ctx,
			"test:index:myComponent", "cp1", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)
	// Both aliases must be present: the user-supplied Name-only one and the
	// auto-generated URN-scoped one for backwards compatibility with un-prefixed stacks.
	assert.Equal(t, []string{"legacyName"}, aliasNames)
	assert.Equal(t,
		[]string{"urn:pulumi:stackDev::projectFoo::test:index:myComponent$" + testResourceToken + "::inner"},
		aliasURNs,
	)
}

// TestComponentResourceProviderAndRead verifies that the prefix logic also applies on
// the provider-resource path (pulumi:providers:*) and the read path (resources with
// `get.id`), since both share the same `resourceName` variable in registerResource.
func TestComponentResourceProviderAndRead(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      myProvider:
        type: pulumi:providers:test
      readMe:
        type: ` + testResourceToken + `
        get:
          id: external-id
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	type observation struct {
		name      string
		read      bool
		aliasURNs []string
	}
	var observed []observation
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case "pulumi:providers:test", testResourceToken:
				obs := observation{
					name: args.Name,
					read: args.ReadRPC != nil,
				}
				if args.RegisterRPC != nil {
					for _, a := range args.RegisterRPC.Aliases {
						if u := a.GetUrn(); u != "" {
							obs.aliasURNs = append(obs.aliasURNs, u)
						}
					}
				}
				observed = append(observed, obs)
				return "id", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, _, err := RunComponentTemplate(ctx,
			"test:index:myComponent", "cp1", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)

	var providerObs, readObs *observation
	for i := range observed {
		switch observed[i].name {
		case "cp1-myProvider":
			providerObs = &observed[i]
		case "cp1-readMe":
			readObs = &observed[i]
		}
	}
	require.NotNil(t, providerObs, "expected provider resource registered as cp1-myProvider; got %+v", observed)
	require.NotNil(t, readObs, "expected read resource registered as cp1-readMe; got %+v", observed)
	assert.False(t, providerObs.read, "provider resource should not be a read")
	assert.Equal(t,
		[]string{"urn:pulumi:stackDev::projectFoo::test:index:myComponent$pulumi:providers:test::myProvider"},
		providerObs.aliasURNs,
	)
	assert.True(t, readObs.read, "readMe should go through the read path (Get.Id was set)")
}

// TestComponentResourceAliasSuppressedOnSiblingCollision verifies that when a child's
// post-fix name would equal another sibling's literal pre-fix name, the auto-alias is
// suppressed for that child to avoid emitting an alias whose target URN duplicates the
// sibling's current URN — which the engine would reject as an ambiguous claim. The
// sibling's own alias still gets emitted.
//
// Component "Outer" with siblings "child" and "res-child", instantiated as "res":
//   - "child"     → post-fix "res-child"     ; alias would target "::child"
//   - "res-child" → post-fix "res-res-child" ; alias targets "::res-child"
//
// "child" post-fix URN matches "res-child"'s pre-fix URN, so "child"'s alias must
// be suppressed.
func TestComponentResourceAliasSuppressedOnSiblingCollision(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
components:
  myComponent:
    resources:
      child:
        type: ` + testResourceToken + `
        properties:
          foo: bar
      res-child:
        type: ` + testResourceToken + `
        properties:
          foo: bar
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	type observation struct {
		name      string
		aliasURNs []string
	}
	var observed []observation
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			switch args.TypeToken {
			case "test:index:myComponent":
				return "", resource.PropertyMap{}, nil
			case testResourceToken:
				obs := observation{name: args.Name}
				if args.RegisterRPC != nil {
					for _, a := range args.RegisterRPC.Aliases {
						if u := a.GetUrn(); u != "" {
							obs.aliasURNs = append(obs.aliasURNs, u)
						}
					}
				}
				observed = append(observed, obs)
				return "id", resource.PropertyMap{
					"foo": resource.NewStringProperty("bar"),
					"bar": resource.NewStringProperty("baz"),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, _, err := RunComponentTemplate(ctx,
			"test:index:myComponent", "res", nil,
			template, pulumi.Map{}, newMockPackageMap(),
		)
		return err
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		requireNoErrors(t, template, diags)
	}
	require.NoError(t, err)

	var childObs, resChildObs *observation
	for i := range observed {
		switch observed[i].name {
		case "res-child":
			childObs = &observed[i]
		case "res-res-child":
			resChildObs = &observed[i]
		}
	}
	require.NotNil(t, childObs, "expected post-fix `res-child` (from `child`); got %+v", observed)
	require.NotNil(t, resChildObs, "expected post-fix `res-res-child` (from `res-child`); got %+v", observed)

	// `child` (post-fix `res-child`) must NOT carry an alias — its target URN
	// would collide with `res-child` sibling's pre-fix URN.
	assert.Empty(t, childObs.aliasURNs,
		"child's alias must be suppressed to avoid colliding with sibling's pre-fix URN")

	// `res-child` (post-fix `res-res-child`) keeps its alias to the un-prefixed URN.
	assert.Equal(t,
		[]string{"urn:pulumi:stackDev::projectFoo::test:index:myComponent$" + testResourceToken + "::res-child"},
		resChildObs.aliasURNs,
	)
}
