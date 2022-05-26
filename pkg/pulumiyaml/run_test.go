// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

const testComponentToken = "test:component:type"
const testResourceToken = "test:resource:type"

type MockPackageLoader struct {
	packages map[string]Package
}

func (m MockPackageLoader) LoadPackage(name string) (Package, error) {
	if pkg, found := m.packages[name]; found {
		return pkg, nil
	}
	return nil, fmt.Errorf("package not found")
}

func (m MockPackageLoader) Close() {}

type MockPackage struct {
	isComponent      func(typeName string) (bool, error)
	resolveResource  func(typeName string) (ResourceTypeToken, error)
	resolveFunction  func(typeName string) (FunctionTypeToken, error)
	resourceTypeHint func(typeName string) *schema.ResourceType
	functionTypeHint func(typeName string) *schema.Function
}

func (m MockPackage) ResolveResource(typeName string) (ResourceTypeToken, error) {
	if m.resolveResource != nil {
		return m.resolveResource(typeName)
	}
	return ResourceTypeToken(typeName), nil
}

func (m MockPackage) ResolveFunction(typeName string) (FunctionTypeToken, error) {
	if m.resolveFunction != nil {
		return m.resolveFunction(typeName)
	}
	return FunctionTypeToken(typeName), nil
}

func (m MockPackage) IsComponent(typeName ResourceTypeToken) (bool, error) {
	return m.isComponent(typeName.String())
}

func (m MockPackage) ResourceTypeHint(typeName ResourceTypeToken) *schema.ResourceType {
	return m.resourceTypeHint(typeName.String())
}

func (m MockPackage) FunctionTypeHint(typeName FunctionTypeToken) *schema.Function {
	return m.functionTypeHint(typeName.String())
}

func (m MockPackage) ResourceConstants(typeName ResourceTypeToken) map[string]interface{} {
	return nil
}

func (m MockPackage) Name() string {
	return "test"
}

func inputProperties(token string, props ...schema.Property) *schema.ResourceType {
	p := make([]*schema.Property, 0, len(props))
	for _, prop := range props {
		prop := prop
		p = append(p, &prop)
	}
	return &schema.ResourceType{
		Resource: &schema.Resource{
			InputProperties: p,
			Properties:      p,
		},
	}
}

func function(token string, inputs, outputs []schema.Property) *schema.Function {
	pIn := make([]*schema.Property, 0, len(inputs))
	pOut := make([]*schema.Property, 0, len(outputs))
	for _, prop := range inputs {
		prop := prop
		pIn = append(pIn, &prop)
	}
	for _, prop := range outputs {
		prop := prop
		pOut = append(pOut, &prop)
	}
	return &schema.Function{
		Token:   testComponentToken,
		Inputs:  &schema.ObjectType{Properties: pIn},
		Outputs: &schema.ObjectType{Properties: pOut},
	}
}

func newMockPackageMap() PackageLoader {
	return MockPackageLoader{
		packages: map[string]Package{
			"aws": MockPackage{},
			"test": MockPackage{
				resourceTypeHint: func(typeName string) *schema.ResourceType {
					switch typeName {
					case testResourceToken:
						return inputProperties(typeName, schema.Property{
							Name: "foo",
							Type: schema.StringType,
						}, schema.Property{
							Name: "bar",
							Type: schema.StringType,
						})
					case testComponentToken:
						return inputProperties(typeName, schema.Property{
							Name: "foo",
							Type: schema.StringType,
						})
					default:
						return inputProperties(typeName)
					}
				},
				functionTypeHint: func(typeName string) *schema.Function {
					switch typeName {
					case "test:fn":
						return function(typeName,
							[]schema.Property{
								{Name: "yesArg", Type: schema.StringType},
								{Name: "someSuchArg", Type: schema.StringType},
							},
							[]schema.Property{
								{Name: "outString", Type: schema.StringType},
							})
					default:
						return function(typeName, nil, nil)
					}
				},
				isComponent: func(typeName string) (bool, error) {
					switch typeName {
					case testResourceToken:
						return false, nil
					case testComponentToken:
						return true, nil
					default:
						// TODO: Remove this and fix all test cases.
						return false, nil
					}
				},
			},
		}}
}

type testMonitor struct {
	CallF        func(args pulumi.MockCallArgs) (resource.PropertyMap, error)
	NewResourceF func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error)
}

func (m *testMonitor) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	if m.CallF == nil {
		return resource.PropertyMap{}, nil
	}
	return m.CallF(args)
}

func (m *testMonitor) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if m.NewResourceF == nil {
		return args.Name, resource.PropertyMap{}, nil
	}
	return m.NewResourceF(args)
}

const testProject = "foo"

func projectConfigKey(k string) resource.PropertyKey {
	return resource.PropertyKey(testProject + ":" + k)
}

func setConfig(t *testing.T, m resource.PropertyMap) {
	config := m.Mappable()
	b, err := json.Marshal(config)
	require.NoError(t, err, "Failed to marshal the map")
	t.Setenv(pulumi.EnvConfig, string(b))
	if m.ContainsSecrets() {
		var secrets []string
		for k, v := range m {
			if v.IsSecret() {
				secrets = append(secrets, string(k))
				t.Logf("Found secret: '%s': %v <== %v", string(k), v, secrets)
			}
		}
		t.Logf("Setting secret keys = %v", secrets)
		s, err := json.Marshal(secrets)
		require.NoError(t, err, "Failed to marshal secrets")
		t.Setenv(pulumi.EnvConfigSecretKeys, string(s))
	}
}

func testTemplateDiags(t *testing.T, template *ast.TemplateDecl, callback func(*evalContext)) syntax.Diagnostics {
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {

			switch args.TypeToken {
			case testResourceToken:
				assert.Equal(t, resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				}), args.Inputs, "expected resource test:resource:type to have property foo: oof")
				assert.Equal(t, "", args.Provider)
				assert.Equal(t, "", args.ID)

				return "someID", resource.PropertyMap{
					"foo":    resource.NewStringProperty("qux"),
					"bar":    resource.NewStringProperty("oof"),
					"out":    resource.NewStringProperty("tuo"),
					"outSep": resource.NewStringProperty("1-2-3-4"),
					"outNum": resource.NewNumberProperty(1),
					"outList": resource.NewPropertyValue([]interface{}{
						map[string]interface{}{
							"value": 42,
						},
						map[string]interface{}{
							"value": 24,
						},
					}),
				}, nil
			case testComponentToken:
				assert.Equal(t, testComponentToken, args.TypeToken)
				assert.True(t, args.Inputs.DeepEquals(resource.NewPropertyMapFromMap(map[string]interface{}{
					"foo": "oof",
				})))
				assert.Equal(t, "", args.Provider)
				assert.Equal(t, "", args.ID)

				return "", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template, newMockPackageMap())
		_, diags := TypeCheck(runner)
		if diags.HasErrors() {
			return diags
		}
		err := runner.Evaluate()
		if err.HasErrors() {
			return err
		}
		if callback != nil {
			ctx := runner.newContext(nil)
			callback(ctx)
		}
		return nil
	}, pulumi.WithMocks(testProject, "dev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		return diags
	}
	assert.NoError(t, err)
	return nil
}

func testTemplateSyntaxDiags(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) syntax.Diagnostics {
	// Same mocks as in testTemplateDiags but without assertions, just pure syntax checking.
	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {

			switch args.TypeToken {
			case testResourceToken:
				return "someID", resource.PropertyMap{
					"foo":    resource.NewStringProperty("qux"),
					"bar":    resource.NewStringProperty("oof"),
					"out":    resource.NewStringProperty("tuo"),
					"outNum": resource.NewNumberProperty(1),
				}, nil
			case testComponentToken:
				return "", resource.PropertyMap{}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template, newMockPackageMap())
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

func testTemplate(t *testing.T, template *ast.TemplateDecl, callback func(*evalContext)) {
	diags := testTemplateDiags(t, template, callback)
	requireNoErrors(t, template, diags)
}

func TestYAML(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  comp-a:
    type: test:component:type
    properties:
      foo: ${res-a.bar}
outputs:
  foo: ${res-a.foo}
  bar: ${res-a}
`
	tmpl := yamlTemplate(t, text)
	testTemplate(t, tmpl, func(r *evalContext) {})
}

func TestAssetOrArchive(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
variables:
  dir:
    Fn::AssetArchive:
      str:
        Fn::StringAsset: this is home
      away:
        Fn::RemoteAsset: example.org/asset
      local:
        Fn::FileAsset: ./asset
      folder:
        Fn::AssetArchive:
          docs:
            Fn::RemoteArchive: example.org/docs
`
	tmpl := yamlTemplate(t, text)
	testTemplate(t, tmpl, func(ctx *evalContext) {
		dir, ok := ctx.variables["dir"]
		require.True(t, ok, "must have found dir")
		assetArchive, ok := dir.(pulumi.Archive)
		require.True(t, ok)

		assets := assetArchive.Assets()
		assert.Equal(t, assets["str"].(pulumi.Asset).Text(), "this is home")
		assert.Equal(t, assets["away"].(pulumi.Asset).URI(), "example.org/asset")
		assert.Equal(t, assets["local"].(pulumi.Asset).Path(), "./asset")
		assert.Equal(t, assets["folder"].(pulumi.Archive).Assets()["docs"].(pulumi.Archive).URI(), "example.org/docs")
	})
}

func TestPropertiesAbsent(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateSyntaxDiags(t, tmpl, func(r *runner) {})
	require.Len(t, diags, 0)
	// Consider warning on this?
	// require.True(t, diags.HasErrors())
	// assert.Equal(t, "<stdin>:4:3: resource res-a passed has an empty properties value", diagString(diags[0]))
}

func TestYAMLDiags(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
outputs:
  out: ${res-b}
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	require.True(t, diags.HasErrors())
	assert.Len(t, diags, 1)
	assert.Equal(t, `<stdin>:9:8: resource or variable named "res-b" could not be found`, diagString(diags[0]))
}

func TestConfigTypes(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
configuration:
  foo:
    type: String
    default: 42
  bar: {}
  fizz:
    default: 42
  buzz:
    type: List<String>
  fizzBuzz:
    default: [ "fizz", "buzz" ]
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	assert.Contains(t, diagStrings,
		"<stdin>:4:3: type mismatch: default value of type Number but type String was specified")
	assert.Contains(t, diagStrings,
		"<stdin>:7:3: unable to infer type: either 'default' or 'type' is required")
	assert.Contains(t, diagStrings,
		"<stdin>:10:3: missing required configuration variable 'buzz'; run `pulumi config` to set")
	assert.Len(t, diagStrings, 3)
	require.True(t, diags.HasErrors())
}

func TestConfigSecrets(t *testing.T) { //nolint:paralleltest
	const text = `name: test-yaml
runtime: yaml
configuration:
  foo:
    secret: true
    type: Number
  bar:
    type: String
  fizz:
    default: 42
  buzz:
    default: 42
    secret: true
`

	tmpl := yamlTemplate(t, text)
	setConfig(t,
		resource.PropertyMap{
			projectConfigKey("foo"): resource.NewStringProperty("42.0"),
			projectConfigKey("bar"): resource.MakeSecret(resource.NewStringProperty("the answer")),
		})
	testRan := false
	err := testTemplateDiags(t, tmpl, func(r *evalContext) {

		// Secret because declared secret in configuration
		assert.True(t, pulumi.IsSecret(r.config["foo"].(pulumi.Output)))
		// Secret because declared secret in in config
		assert.True(t, pulumi.IsSecret(r.config["bar"].(pulumi.Output)))
		// Secret because declared secret in configuration (& default)
		assert.True(t, pulumi.IsSecret(r.config["buzz"].(pulumi.Output)))
		// not secret
		assert.Equal(t, 42.0, r.config["fizz"])

		testRan = true
	})
	assert.True(t, testRan, "Our tests didn't run")
	diags, found := HasDiagnostics(err)
	assert.False(t, found, "We should not get any errors: '%s'", diags)
}

func TestConflictingConfigSecrets(t *testing.T) { //nolint:paralleltest
	const text = `name: test-yaml
runtime: yaml
configuration:
  foo:
    secret: false
    type: Number
`

	tmpl := yamlTemplate(t, text)
	setConfig(t,
		resource.PropertyMap{
			projectConfigKey("foo"): resource.MakeSecret(resource.NewStringProperty("42.0")),
		})
	diags := testTemplateDiags(t, tmpl, nil)
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}

	assert.Contains(t, diagStrings,
		"<stdin>:5:13: Cannot mark a configuration value as not secret if the associated config value is secret")
	assert.Len(t, diagStrings, 1)
	require.True(t, diags.HasErrors())

}

func TestDuplicateKeyDiags(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
configuration:
  foo:
    type: string
  foo:
    type: int
variables:
  bar: 1
  bar: 2
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
  res-a:
    type: test:resource:type
    properties:
      foo: oof
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	assert.Contains(t, diagStrings, "<stdin>:6:3: found duplicate config foo")
	assert.Contains(t, diagStrings, "<stdin>:16:3: found duplicate resource res-a")
	assert.Contains(t, diagStrings, "<stdin>:10:3: found duplicate variable bar")
	assert.Len(t, diagStrings, 3)
	require.True(t, diags.HasErrors())
}

func TestConflictKeyDiags(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
configuration:
  foo:
    type: string
variables:
  foo: 1
resources:
  foo:
    type: test:resource:type
    properties:
      foo: oof
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	// Config is evaluated first, so we expect errors on the other two.
	assert.Contains(t, diagStrings, "<stdin>:9:3: resource foo cannot have the same name as config foo")
	assert.Contains(t, diagStrings, "<stdin>:7:3: variable foo cannot have the same name as config foo")
	assert.Len(t, diagStrings, 2)
	require.True(t, diags.HasErrors())
}

func TestConflictResourceVarKeyDiags(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
runtime: yaml
variables:
  foo: 1
resources:
  foo:
    type: test:resource:type
    properties:
      foo: oof
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	// Config is evaluated first, so we expect no errors.
	assert.Contains(t, diagStrings, "<stdin>:4:3: variable foo cannot have the same name as resource foo")
	assert.Len(t, diagStrings, 1)
	require.True(t, diags.HasErrors())
}

func TestJSON(t *testing.T) {
	t.Parallel()

	const text = `{
	"name": "test-yaml",
	"runtime": "yaml",
	"resources": {
		"res-a": {
			"type": "test:resource:type",
			"properties": {
				"foo": "oof"
			}
		},
		"comp-a": {
			"type": "test:component:type",
			"properties": {
				"foo": "${res-a.bar}"
			}
		}
	},
	"outputs": {
		"foo": "${res-a.bar}",
		"bar": "${res-a}"
	}
}`

	tmpl := yamlTemplate(t, text)
	testTemplate(t, tmpl, func(r *evalContext) {})
}

func TestJSONDiags(t *testing.T) {
	t.Parallel()

	const text = `{
	"name": "test-yaml",
	"runtime": "yaml",
	"resources": {
		"res-a": {
			"type": "test:resource:type",
			"properties": {
				"foo": "oof"
			}
		}
	},
	"outputs": {
		"foo": "${res-b}"
	}
}
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	require.True(t, diags.HasErrors())
	assert.Len(t, diags, 1)
	assert.Equal(t, `<stdin>:13:10: resource or variable named "res-b" could not be found`, diagString(diags[0]))
}

func TestPropertyAccessVarMap(t *testing.T) {

	t.Parallel()

	const text = `
name: aws-eks
runtime: yaml
description: An EKS cluster
variables:
  test:
    - quux:
        bazz: notoof
    - quux:
        bazz: oof
resources:
  r:
    type: test:resource:type
    properties:
      foo: ${test[1].quux.bazz}
`
	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	requireNoErrors(t, tmpl, diags)
}

func TestSchemaPropertyDiags(t *testing.T) {
	t.Parallel()

	const text = `
name: aws-eks
runtime: yaml
description: An EKS cluster
variables:
  vpcId:
    Fn::Invoke:
      Function: test:fn
      Arguments:
        noArg: false
        yesArg: true
resources:
  r:
    type: test:resource:type
    properties:
      foo: ${vpcId.outString} # order to ensure determinism
      buzz: does not exist
`
	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	require.True(t, diags.HasErrors())
	assert.Len(t, diags, 2)
	assert.Equal(t, "<stdin>:10:9: noArg does not exist on Invoke test:fn",
		diagString(diags[0]))
	assert.Equal(t, "<stdin>:17:7: Property buzz does not exist on Resource test:resource:type",
		diagString(diags[1]))

}

func TestPropertyAccess(t *testing.T) {
	t.Parallel()
	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"resA": {
				Type: "test:resource:type",
				Properties: map[string]interface{}{
					"foo": "oof",
				},
			},
		},
	})
	testTemplate(t, tmpl, func(r *evalContext) {
		x, diags := ast.Interpolate("${resA.outList[0].value}")
		requireNoErrors(t, tmpl, diags)

		v, ok := r.evaluatePropertyAccess(x, x.Parts[0].Value)
		assert.True(t, ok)
		r.ctx.Export("out", pulumi.Any(v))
	})
}

func TestJoin(t *testing.T) {
	t.Parallel()

	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"resA": {
				Type: "test:resource:type",
				Properties: map[string]interface{}{
					"foo": "oof",
				},
			},
		},
	})
	testTemplate(t, tmpl, func(r *evalContext) {
		v, ok := r.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: ast.String(","),
			Values: ast.List(
				ast.String("a"),
				ast.String("b"),
				ast.String("c"),
			),
		})
		assert.True(t, ok)
		assert.Equal(t, "a,b,c", v)

		x, diags := ast.Interpolate("${resA.out}")
		requireNoErrors(t, tmpl, diags)

		v, ok = r.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: x,
			Values: ast.List(
				ast.String("["),
				ast.String("]"),
			),
		})
		assert.True(t, ok)
		out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "[tuo]", x)
			return nil, nil
		})
		r.ctx.Export("out", out)

		v, ok = r.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: ast.String(","),
			Values:    ast.List(x, x),
		})
		assert.True(t, ok)
		out = v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "tuo,tuo", x)
			return nil, nil
		})
		r.ctx.Export("out2", out)
	})
}

func TestSplit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    *ast.SplitExpr
		expected []string
		isOutput bool
	}{
		{
			input: &ast.SplitExpr{
				Delimiter: ast.String(","),
				Source:    ast.String("a,b"),
			},
			expected: []string{"a", "b"},
		},
		{
			input: &ast.SplitExpr{
				Delimiter: ast.String(","),
				Source:    ast.String("a"),
			},
			expected: []string{"a"},
		},
		{
			input: &ast.SplitExpr{
				Delimiter: ast.String(","),
				Source:    ast.String(""),
			},
			expected: []string{""},
		},
		{
			input: &ast.SplitExpr{
				Source: &ast.SymbolExpr{
					Property: &ast.PropertyAccess{
						Accessors: []ast.PropertyAccessor{
							&ast.PropertyName{Name: "resA"},
							&ast.PropertyName{Name: "outSep"},
						},
					},
				},
				Delimiter: ast.String("-"),
			},
			expected: []string{"1", "2", "3", "4"},
			isOutput: true,
		},
	}
	//nolint:paralleltest // false positive that the "tt" var isn't used, it is via "tt.expected"
	for _, tt := range tests {
		tt := tt
		t.Run(strings.Join(tt.expected, ","), func(t *testing.T) {
			t.Parallel()

			tmpl := template(t, &Template{
				Resources: map[string]*Resource{
					"resA": {
						Type: "test:resource:type",
						Properties: map[string]interface{}{
							"foo": "oof",
						},
					},
				},
			})
			testTemplate(t, tmpl, func(ctx *evalContext) {
				v, ok := ctx.evaluateBuiltinSplit(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						assert.Equal(t, tt.expected, x)
						return nil, nil
					})
					ctx.ctx.Export("out", out)
				} else {
					assert.Equal(t, tt.expected, v)
				}
			})
		})
	}
}

func TestToJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    *ast.ToJSONExpr
		expected string
		isOutput bool
	}{
		{
			input: &ast.ToJSONExpr{
				Value: ast.List(
					ast.String("a"),
					ast.String("b"),
				),
			},
			expected: `["a","b"]`,
		},
		{
			input: &ast.ToJSONExpr{
				Value: ast.Object(
					ast.ObjectProperty{
						Key:   ast.String("one"),
						Value: ast.Number(1),
					},
					ast.ObjectProperty{
						Key:   ast.String("two"),
						Value: ast.List(ast.Number(1), ast.Number(2)),
					},
				),
			},
			expected: `{"one":1,"two":[1,2]}`,
		},
		{
			input: &ast.ToJSONExpr{
				Value: ast.List(
					&ast.JoinExpr{
						Delimiter: ast.String("-"),
						Values: ast.List(
							ast.String("a"),
							ast.String("b"),
							ast.String("c"),
						),
					}),
			},
			expected: `["a-b-c"]`,
		},
		{
			input: &ast.ToJSONExpr{
				Value: ast.Object(
					ast.ObjectProperty{
						Key:   ast.String("foo"),
						Value: ast.String("bar"),
					},
					ast.ObjectProperty{
						Key: ast.String("out"),
						Value: &ast.SymbolExpr{
							Property: &ast.PropertyAccess{
								Accessors: []ast.PropertyAccessor{
									&ast.PropertyName{Name: "resA"},
									&ast.PropertyName{Name: "out"},
								},
							},
						},
					}),
			},
			expected: `{"foo":"bar","out":"tuo"}`,
			isOutput: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			tmpl := template(t, &Template{
				Resources: map[string]*Resource{
					"resA": {
						Type: "test:resource:type",
						Properties: map[string]interface{}{
							"foo": "oof",
						},
					},
				},
			})
			testTemplate(t, tmpl, func(r *evalContext) {
				v, ok := r.evaluateBuiltinToJSON(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						assert.Equal(t, tt.expected, x)
						return nil, nil
					})
					r.ctx.Export("out", out)
				} else {
					assert.Equal(t, tt.expected, v)
				}
			})
		})
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    *ast.SelectExpr
		expected interface{}
		isOutput bool
		isError  bool
	}{
		{
			input: &ast.SelectExpr{
				Index: ast.Number(1),
				Values: ast.List(
					ast.Number(1),
					ast.String("second"),
				),
			},
			expected: "second",
		},
		{
			input: &ast.SelectExpr{
				Index: ast.Number(0),
				Values: &ast.SymbolExpr{
					Property: &ast.PropertyAccess{
						Accessors: []ast.PropertyAccessor{
							&ast.PropertyName{Name: "resA"},
							&ast.PropertyName{Name: "outList"},
						},
					},
				},
			},
			expected: map[string]interface{}{"value": 42.0},
			isOutput: true,
		},
		{
			input: &ast.SelectExpr{
				Index: &ast.SymbolExpr{
					Property: &ast.PropertyAccess{
						Accessors: []ast.PropertyAccessor{
							&ast.PropertyName{Name: "resA"},
							&ast.PropertyName{Name: "outNum"},
						},
					},
				},
				Values: ast.List(
					ast.String("first"),
					ast.String("second"),
					ast.String("third"),
				),
			},
			expected: "second",
			isOutput: true,
		},
		{
			input: &ast.SelectExpr{
				Index: ast.Number(1.5),
				Values: ast.List(
					ast.String("first"),
					ast.String("second"),
					ast.String("third"),
				),
			},
			isError: true,
		},
		{
			input: &ast.SelectExpr{
				Index: ast.Number(3),
				Values: ast.List(
					ast.String("first"),
					ast.String("second"),
					ast.String("third"),
				),
			},
			isError: true,
		},
		{
			input: &ast.SelectExpr{
				Index: ast.Number(-182),
				Values: ast.List(
					ast.String("first"),
					ast.String("second"),
					ast.String("third"),
				),
			},
			isError: true,
		},
	}
	//nolint:paralleltest // false positive that the "dir" var isn't used, it is via idx
	for idx, tt := range tests {
		tt := tt
		if idx != 4 {
			continue
		}
		t.Run(fmt.Sprint(idx), func(t *testing.T) {
			t.Parallel()

			tmpl := template(t, &Template{
				Resources: map[string]*Resource{
					"resA": {
						Type: testResourceToken,
						Properties: map[string]interface{}{
							"foo": "oof",
						},
					},
				},
			})
			testTemplate(t, tmpl, func(ctx *evalContext) {
				v, ok := ctx.evaluateBuiltinSelect(tt.input)
				if tt.isError {
					assert.False(t, ok)
					assert.True(t, ctx.sdiags.HasErrors())
					assert.Nil(t, v)
					return
				}

				requireNoErrors(t, tmpl, ctx.sdiags.diags)
				if tt.isOutput {
					out := v.(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
						assert.Equal(t, tt.expected, x)
						return nil, nil
					})
					ctx.ctx.Export("out", out)
				} else {
					assert.Equal(t, tt.expected, v)
				}

			})
		})
	}
}

func TestFromBase64ErrorOnInvalidUTF8(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input *ast.FromBase64Expr
		name  string
		valid bool
	}{
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("a"))),
			},
			name:  "Valid ASCII",
			valid: true,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xc3\xb1"))),
			},
			name:  "Valid 2 Octet Sequence",
			valid: true,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xe2\x82\xa1"))),
			},
			name:  "Valid 3 Octet Sequence",
			valid: true,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xf0\x90\x8c\xbc"))),
			},
			name:  "Valid 4 Octet Sequence",
			valid: true,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xf8\xa1\xa1\xa1\xa1"))),
			},
			name:  "Valid 5 Octet Sequence (but not Unicode!)",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xfc\xa1\xa1\xa1\xa1\xa1"))),
			},
			name:  "Valid 6 Octet Sequence (but not Unicode!)",
			valid: false,
		},

		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xfc\xa1\xa1\xa1\xa1\xa1"))),
			},
			name:  "Valid 6 Octet Sequence (but not Unicode!)",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xc3\x28"))),
			},
			name:  "Invalid 2 Octet Sequence",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xa0\xa1"))),
			},
			name:  "Invalid Sequence Identifier",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xe2\x28\xa1"))),
			},
			name:  "Invalid 3 Octet Sequence (in 2nd Octet)",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xe2\x82\x28"))),
			},
			name:  "Invalid 3 Octet Sequence (in 3rd Octet)",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xf0\x28\x8c\xbc"))),
			},
			name:  "Invalid 4 Octet Sequence (in 2nd Octet)",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xf0\x90\x28\xbc"))),
			},
			name:  "Invalid 4 Octet Sequence (in 3rd Octet)",
			valid: false,
		},
		{
			input: &ast.FromBase64Expr{
				Value: ast.String(b64.StdEncoding.EncodeToString([]byte("\xf0\x28\x8c\x28"))),
			},
			name:  "Invalid 4 Octet Sequence (in 4th Octet)",
			valid: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpl := template(t, &Template{
				Resources: map[string]*Resource{},
			})
			testTemplate(t, tmpl, func(r *evalContext) {
				_, ok := r.evaluateBuiltinFromBase64(tt.input)
				assert.Equal(t, tt.valid, ok)
			})
		})
	}
}

func TestBase64Roundtrip(t *testing.T) {
	t.Parallel()

	tToFrom := struct {
		input    *ast.ToBase64Expr
		expected string
	}{
		input: &ast.ToBase64Expr{
			Value: &ast.FromBase64Expr{
				Value: ast.String("SGVsbG8sIFdvcmxk"),
			},
		},
		expected: "SGVsbG8sIFdvcmxk",
	}

	t.Run(tToFrom.expected, func(t *testing.T) {
		t.Parallel()

		tmpl := template(t, &Template{
			Resources: map[string]*Resource{},
		})
		testTemplate(t, tmpl, func(r *evalContext) {
			v, ok := r.evaluateBuiltinToBase64(tToFrom.input)
			assert.True(t, ok)
			assert.Equal(t, tToFrom.expected, v)
		})
	})

	tFromTo := struct {
		input    *ast.FromBase64Expr
		expected string
	}{
		input: &ast.FromBase64Expr{
			Value: &ast.ToBase64Expr{
				Value: ast.String("Hello, World!"),
			},
		},
		expected: "Hello, World!",
	}

	t.Run(tFromTo.expected, func(t *testing.T) {
		t.Parallel()

		tmpl := template(t, &Template{
			Resources: map[string]*Resource{},
		})
		testTemplate(t, tmpl, func(r *evalContext) {
			v, ok := r.evaluateBuiltinFromBase64(tFromTo.input)
			assert.True(t, ok)
			assert.Equal(t, tFromTo.expected, v)
		})
	})
}

func TestFromBase64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    *ast.FromBase64Expr
		expected string
		isOutput bool
	}{
		{
			input: &ast.FromBase64Expr{
				Value: ast.String("dGhpcyBpcyBhIHRlc3Q="),
			},
			expected: "this is a test",
		},
		{
			input: &ast.FromBase64Expr{
				Value: &ast.JoinExpr{
					Delimiter: ast.String(""),
					Values: ast.List(
						ast.String("My4xN"),
						ast.String("DE1OTI="),
					),
				}},
			expected: "3.141592",
		},
		{
			input: &ast.FromBase64Expr{
				Value: &ast.ToBase64Expr{
					Value: ast.String("test"),
				},
			},
			expected: "test",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			tmpl := template(t, &Template{
				Resources: map[string]*Resource{
					"resA": {
						Type: "test:resource:type",
						Properties: map[string]interface{}{
							"foo": "oof",
						},
					},
				},
			})
			testTemplate(t, tmpl, func(r *evalContext) {
				v, ok := r.evaluateBuiltinFromBase64(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						s := b64.StdEncoding.EncodeToString([]byte(tt.expected))
						assert.Equal(t, s, v)
						return nil, nil
					})
					r.ctx.Export("out", out)
				} else {
					assert.Equal(t, tt.expected, v)
				}
			})
		})
	}

}

func TestToBase64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    *ast.ToBase64Expr
		expected string
		isOutput bool
	}{
		{
			input: &ast.ToBase64Expr{
				Value: ast.String("this is a test"),
			},
			expected: "this is a test",
		},
		{
			input: &ast.ToBase64Expr{
				Value: &ast.JoinExpr{
					Delimiter: ast.String("."),
					Values: ast.List(
						ast.String("3"),
						ast.String("141592"),
					),
				}},
			expected: "3.141592",
		},
		{
			input: &ast.ToBase64Expr{
				Value: &ast.SymbolExpr{
					Property: &ast.PropertyAccess{
						Accessors: []ast.PropertyAccessor{
							&ast.PropertyName{Name: "resA"},
							&ast.PropertyName{Name: "out"},
						},
					},
				},
			},
			expected: "tuo",
			isOutput: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			tmpl := template(t, &Template{
				Resources: map[string]*Resource{
					"resA": {
						Type: "test:resource:type",
						Properties: map[string]interface{}{
							"foo": "oof",
						},
					},
				},
			})
			testTemplate(t, tmpl, func(r *evalContext) {
				v, ok := r.evaluateBuiltinToBase64(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						s, err := b64.StdEncoding.DecodeString(x.(string))
						assert.NoError(t, err)
						assert.Equal(t, tt.expected, string(s))
						return nil, nil
					})
					r.ctx.Export("out", out)
				} else {
					s, err := b64.StdEncoding.DecodeString(v.(string))
					assert.NoError(t, err)
					assert.Equal(t, tt.expected, string(s))
				}
			})
		})
	}

}

func TestSub(t *testing.T) {
	t.Parallel()

	tmpl := template(t, &Template{
		Variables: map[string]interface{}{
			"foo": "oof",
		},
		Resources: map[string]*Resource{
			"resA": {
				Type: testResourceToken,
				Properties: map[string]interface{}{
					"foo": "oof",
				},
			},
		},
	})
	testTemplate(t, tmpl, func(r *evalContext) {
		v, ok := r.evaluateInterpolate(ast.MustInterpolate("Hello ${foo}!"))
		assert.True(t, ok)
		assert.Equal(t, "Hello oof!", v)

		v, ok = r.evaluateInterpolate(ast.MustInterpolate("Hello ${resA.out} - ${resA.id}!!"))
		assert.True(t, ok)
		out := v.(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "Hello tuo - someID!!", x)
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
}

func TestSecret(t *testing.T) {
	t.Parallel()

	const text = `
name: test-secret
runtime: yaml
variables:
  mySecret:
    Fn::Secret: my-special-secret
`
	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	var hasRun = false
	testTemplate(t, tmpl, func(r *evalContext) {
		assert.False(t, r.Evaluate().HasErrors())
		s := r.variables["mySecret"].(pulumi.Output)
		require.True(t, pulumi.IsSecret(s))
		out := s.ApplyT(func(x interface{}) (interface{}, error) {
			hasRun = true
			assert.Equal(t, "my-special-secret", x)
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
	assert.True(t, hasRun)
}

func TestUnicodeLogicalName(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
variables:
  "bB-Beta_beta.💜⁉":
    test: oof
resources:
  "aA-Alpha_alpha.\U0001F92F⁉️":
    type: test:resource:type
    properties:
      foo: "${[\"bB-Beta_beta.💜⁉\"].test}"
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, tmpl, diags)
}
