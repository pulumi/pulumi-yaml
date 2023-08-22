// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	_ "embed"
	"os"
	"path/filepath"
	"reflect"

	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	ctypes "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/config"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

//go:embed README.md
var packageReadmeFile string

const testComponentToken = "test:component:type"
const testResourceToken = "test:resource:type"

type MockPackageLoader struct {
	packages map[string]Package
}

func (m MockPackageLoader) LoadPackage(name string, version *semver.Version) (Package, error) {
	if version != nil {
		// See if there is a version specific package
		if pkg, found := m.packages[name+"@"+version.String()]; found {
			return pkg, nil
		}
	}
	if pkg, found := m.packages[name]; found {
		return pkg, nil
	}
	return nil, fmt.Errorf("package not found")
}

func (m MockPackageLoader) Close() {}

type MockPackage struct {
	version          *semver.Version
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

func (m MockPackage) Version() *semver.Version {
	return m.version
}

func inputProperties(token string, props ...schema.Property) *schema.ResourceType {
	p := make([]*schema.Property, 0, len(props))
	for _, prop := range props {
		prop := prop
		p = append(p, &prop)
	}
	return &schema.ResourceType{
		Resource: &schema.Resource{
			Token:           token,
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
	version := func(tag string) *semver.Version {
		v := semver.MustParse(tag)
		return &v
	}
	return MockPackageLoader{
		packages: map[string]Package{
			"aws": MockPackage{},
			"docker": MockPackage{
				version: version("4.0.0"),
				resourceTypeHint: func(typeName string) *schema.ResourceType {
					return inputProperties(typeName)
				},
			},
			"docker@3.0.0": MockPackage{
				version: version("3.0.0"),
			},
			"test": MockPackage{
				resourceTypeHint: func(typeName string) *schema.ResourceType {
					switch typeName {
					case testResourceToken:
						return inputProperties(typeName, schema.Property{
							Name: "foo",
							Type: schema.StringType,
						}, schema.Property{
							Name: "bar",
							Type: &schema.OptionalType{ElementType: schema.StringType},
						})
					case testComponentToken:
						return inputProperties(typeName, schema.Property{
							Name: "foo",
							Type: schema.StringType,
						})
					case "test:resource:not-run":
						return inputProperties("test:resource:not-run", schema.Property{
							Name: "foo",
							Type: schema.StringType,
						})
					case "test:read:Resource":
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
								{Name: "someSuchArg", Type: &schema.OptionalType{ElementType: schema.StringType}},
							},
							[]schema.Property{
								{Name: "outString", Type: schema.StringType},
							})
					case "test:invoke:poison":
						return function("test:invoke:poison",
							[]schema.Property{{Name: "foo", Type: schema.StringType}},
							[]schema.Property{{Name: "value", Type: schema.StringType}})
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

func testTemplateDiags(t *testing.T, template *ast.TemplateDecl, callback func(*programEvaluator)) syntax.Diagnostics {
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
		runner := newRunner(template, newMockPackageMap())
		_, diags := TypeCheck(runner)
		if diags.HasErrors() {
			return diags
		}
		diags = runner.Evaluate(ctx)
		if diags.HasErrors() {
			return diags
		}
		if callback != nil {
			eCtx := runner.newContext(nil)
			callback(&programEvaluator{evalContext: eCtx, pulumiCtx: ctx})
		}
		return nil
	}, pulumi.WithMocks(testProject, "dev", mocks))
	if diags, ok := HasDiagnostics(err); ok {
		return diags
	}
	assert.NoError(t, err)
	return nil
}

func testTemplateSyntaxDiags(t *testing.T, template *ast.TemplateDecl, callback func(*Runner)) syntax.Diagnostics {
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
		runner := newRunner(template, newMockPackageMap())
		err := runner.Evaluate(ctx)
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

func testTemplate(t *testing.T, template *ast.TemplateDecl, callback func(*programEvaluator)) {
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
	testTemplate(t, tmpl, func(e *programEvaluator) {})
}

func TestAssetOrArchive(t *testing.T) {
	t.Parallel()

	const text = `name: test-yaml
variables:
  foo: bar
  foo2: ./README.md
  dir:
    fn::assetArchive:
      str:
        fn::stringAsset: this is home
      strIter:
        fn::stringAsset: start ${foo} end
      away:
        fn::remoteAsset: example.org/asset
      local:
        fn::fileAsset: ${foo2}
      folder:
        fn::assetArchive:
          docs:
            fn::remoteArchive: example.org/docs
`
	tmpl := yamlTemplate(t, text)
	testTemplate(t, tmpl, func(e *programEvaluator) {
		dir, ok := e.variables["dir"]
		require.True(t, ok, "must have found dir")
		assetArchive, ok := dir.(pulumi.Archive)
		require.True(t, ok)

		assets := assetArchive.Assets()
		assert.Equal(t, assets["str"].(pulumi.Asset).Text(), "this is home")
		assert.Equal(t, assets["strIter"].(pulumi.Asset).Text(), "start bar end")
		assert.Equal(t, assets["away"].(pulumi.Asset).URI(), "example.org/asset")
		filePath, err := filepath.Abs("./README.md")
		assert.NoError(t, err)
		assert.Equal(t, assets["local"].(pulumi.Asset).Path(), filePath)
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
	diags := testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	assert.Contains(t, diagStrings,
		"<stdin>:4:3: type mismatch: default value of type number but type string was specified")
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
	err := testTemplateDiags(t, tmpl, func(e *programEvaluator) {
		// Secret because declared secret in configuration
		assert.True(t, pulumi.IsSecret(e.config["foo"].(pulumi.Output)))
		// Secret because declared secret in in config
		assert.True(t, pulumi.IsSecret(e.config["bar"].(pulumi.Output)))
		// Secret because declared secret in configuration (& default)
		assert.True(t, pulumi.IsSecret(e.config["buzz"].(pulumi.Output)))
		// not secret
		assert.Equal(t, 42.0, e.config["fizz"])

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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
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
	testTemplate(t, tmpl, func(e *programEvaluator) {})
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
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
    fn::invoke:
      function: test:fn
      arguments:
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
	diags := testTemplateDiags(t, tmpl, func(e *programEvaluator) {})
	require.Truef(t, diags.HasErrors(), diags.Error())
	assert.Len(t, diags, 2)
	assert.Equal(t, "<stdin>:10:9: noArg does not exist on Invoke test:fn",
		diagString(diags[1]))
	assert.Equal(t, "<stdin>:17:7: Property buzz does not exist on 'test:resource:type'",
		diagString(diags[0]))

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
	testTemplate(t, tmpl, func(e *programEvaluator) {
		x, diags := ast.Interpolate("${resA.outList[0].value}")
		requireNoErrors(t, tmpl, diags)

		v, ok := e.evaluatePropertyAccess(x, x.Parts[0].Value)
		assert.True(t, ok)
		e.pulumiCtx.Export("out", pulumi.Any(v))
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
	testTemplate(t, tmpl, func(e *programEvaluator) {
		v, ok := e.evaluateBuiltinJoin(&ast.JoinExpr{
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

		v, ok = e.evaluateBuiltinJoin(&ast.JoinExpr{
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
		e.pulumiCtx.Export("out", out)

		v, ok = e.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: ast.String(","),
			Values:    ast.List(x, x),
		})
		assert.True(t, ok)
		out = v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "tuo,tuo", x)
			return nil, nil
		})
		e.pulumiCtx.Export("out2", out)
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
			testTemplate(t, tmpl, func(e *programEvaluator) {
				v, ok := e.evaluateBuiltinSplit(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						assert.Equal(t, tt.expected, x)
						return nil, nil
					})
					e.pulumiCtx.Export("out", out)
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
			testTemplate(t, tmpl, func(e *programEvaluator) {
				v, ok := e.evaluateBuiltinToJSON(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						assert.Equal(t, tt.expected, x)
						return nil, nil
					})
					e.pulumiCtx.Export("out", out)
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
			testTemplate(t, tmpl, func(e *programEvaluator) {
				v, ok := e.evaluateBuiltinSelect(tt.input)
				if tt.isError {
					assert.False(t, ok)
					assert.True(t, e.sdiags.HasErrors())
					assert.Nil(t, v)
					return
				}

				requireNoErrors(t, tmpl, e.sdiags.diags)
				if tt.isOutput {
					out := v.(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
						assert.Equal(t, tt.expected, x)
						return nil, nil
					})
					e.pulumiCtx.Export("out", out)
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
			testTemplate(t, tmpl, func(e *programEvaluator) {
				_, ok := e.evaluateBuiltinFromBase64(tt.input)
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
		testTemplate(t, tmpl, func(e *programEvaluator) {
			v, ok := e.evaluateBuiltinToBase64(tToFrom.input)
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
		testTemplate(t, tmpl, func(e *programEvaluator) {
			v, ok := e.evaluateBuiltinFromBase64(tFromTo.input)
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
			testTemplate(t, tmpl, func(e *programEvaluator) {
				v, ok := e.evaluateBuiltinFromBase64(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						s := b64.StdEncoding.EncodeToString([]byte(tt.expected))
						assert.Equal(t, s, v)
						return nil, nil
					})
					e.pulumiCtx.Export("out", out)
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
			testTemplate(t, tmpl, func(e *programEvaluator) {
				v, ok := e.evaluateBuiltinToBase64(tt.input)
				assert.True(t, ok)
				if tt.isOutput {
					out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
						s, err := b64.StdEncoding.DecodeString(x.(string))
						assert.NoError(t, err)
						assert.Equal(t, tt.expected, string(s))
						return nil, nil
					})
					e.pulumiCtx.Export("out", out)
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
	testTemplate(t, tmpl, func(e *programEvaluator) {
		v, ok := e.evaluateInterpolate(ast.MustInterpolate("Hello ${foo}!"))
		assert.True(t, ok)
		assert.Equal(t, "Hello oof!", v)

		v, ok = e.evaluateInterpolate(ast.MustInterpolate("Hello ${resA.out} - ${resA.id}!!"))
		assert.True(t, ok)
		out := v.(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "Hello tuo - someID!!", x)
			return nil, nil
		})
		e.pulumiCtx.Export("out", out)
	})
}

func TestSecret(t *testing.T) {
	t.Parallel()

	const text = `
name: test-secret
runtime: yaml
variables:
  mySecret:
    fn::secret: my-special-secret
`
	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	var hasRun = false
	testTemplate(t, tmpl, func(e *programEvaluator) {
		assert.False(t, e.evalContext.Evaluate(e.pulumiCtx).HasErrors())
		s := e.variables["mySecret"].(pulumi.Output)
		require.True(t, pulumi.IsSecret(s))
		out := s.ApplyT(func(x interface{}) (interface{}, error) {
			hasRun = true
			assert.Equal(t, "my-special-secret", x)
			return nil, nil
		})
		e.pulumiCtx.Export("out", out)
	})
	assert.True(t, hasRun)
}

func TestReadFile(t *testing.T) {
	t.Parallel()

	repoReadmePath, err := filepath.Abs("../../README.md")
	assert.NoError(t, err)

	repoReadmeText, err := os.ReadFile(repoReadmePath)
	assert.NoError(t, err)

	text := fmt.Sprintf(`
name: test-readfile
runtime: yaml
variables:
  textData:
    fn::readFile: ./README.md
  absInDirData:
    fn::readFile: ${pulumi.cwd}/README.md
  absOutOfDirData:
    fn::readFile: %v
`, repoReadmePath)

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	testTemplate(t, tmpl, func(e *programEvaluator) {
		diags := e.evalContext.Evaluate(e.pulumiCtx)
		requireNoErrors(t, tmpl, diags)
		result, ok := e.variables["textData"].(string)
		assert.True(t, ok)
		assert.Equal(t, packageReadmeFile, result)

		result, ok = e.variables["absInDirData"].(string)
		assert.True(t, ok)
		assert.Equal(t, packageReadmeFile, result)

		result, ok = e.variables["absOutOfDirData"].(string)
		assert.True(t, ok)
		assert.Equal(t, string(repoReadmeText), result)
	})
}

// TestReadFileForbidsPathTraversal ensures that we forbid certain malicious path behaviors which
// allow escaping the project directory in static YAML.
//
// The example program uses a non-constant path which escapes the project directory.
//
// Non-constant paths which read from the project dir are considered safe, likely as uses of
// ${pulumi.cwd}, see above. Constant, absolute path are also permitted, sometimes necessary to use
// a secret or token.
func TestReadFileForbidsPathTraversal(t *testing.T) {
	t.Parallel()

	text := `
name: test-readfile
runtime: yaml
outputs:
  readme:
    fn::readFile: ${pulumi.cwd}/../../go.mod # imagine this is /etc/shadow, /var/run/secrets/tokens, etc.
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})

	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	assert.ElementsMatch(t, diagStrings,
		[]string{
			"<stdin>:5:5: Argument must be a constant or contained in the project dir",
		},
	)
}

func TestJoinTemplate(t *testing.T) {
	t.Parallel()

	text := `
name: test-readfile
runtime: yaml
variables:
  inputs:
    - "foo"
    - "bar"
  foo-bar:
    fn::join:
      - "-"
      - ${inputs}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	testTemplate(t, tmpl, func(e *programEvaluator) {
		diags := e.evalContext.Evaluate(e.pulumiCtx)
		requireNoErrors(t, tmpl, diags)
		result, ok := e.variables["foo-bar"].(string)
		assert.True(t, ok)
		assert.Equal(t, "foo-bar", result)
	})
}

func TestEscapingInterpolationInTemplate(t *testing.T) {
	t.Parallel()

	text := `
name: test-readfile
runtime: yaml
variables:
    world: world
    interpolated: hello ${world}!
    escaped: hello $${world}!
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	testTemplate(t, tmpl, func(e *programEvaluator) {
		diags := e.evalContext.Evaluate(e.pulumiCtx)
		requireNoErrors(t, tmpl, diags)
		result, ok := e.variables["interpolated"].(string)
		assert.True(t, ok)
		assert.Equal(t, "hello world!", result)

		result, ok = e.variables["escaped"].(string)
		assert.True(t, ok)
		assert.Equal(t, "hello ${world}!", result)
	})
}

func TestJoinForbidsNonStringArgs(t *testing.T) {
	t.Parallel()

	text := `
name: test-readfile
runtime: yaml
variables:
  inputs:
    - 1
    - { "foo": "bar" }
    - [1, 2, 3]
    - true
  foo-bar:
    fn::join:
      - "-"
      - ${inputs}
  foo-err:
    fn::join:
      - "-"
      - ${inputs[1]}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})

	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	assert.ElementsMatch(t, diagStrings,
		[]string{
			"<stdin>:12:9: the second argument to fn::join must be a list of strings, found a number at index 0",
			"<stdin>:12:9: the second argument to fn::join must be a list of strings, found an object at index 1",
			"<stdin>:12:9: the second argument to fn::join must be a list of strings, found a list at index 2",
			"<stdin>:12:9: the second argument to fn::join must be a list of strings, found a boolean at index 3",
			"<stdin>:16:9: the second argument to fn::join must be a list, found an object",
		},
	)
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
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestPoisonResult(t *testing.T) {
	t.Parallel()

	text := `
name: test-poison
runtime: yaml
variables:
  poisoned:
    fn::invoke:
      function: test:invoke:poison
      arguments:
        foo: three
      return: value
  never-run:
    fn::invoke:
      function: test:invoke:poison
      arguments:
        foo: ${poisoned}
      return: value
resources:
  alsoPoisoned:
    type: test:resource:not-run
    properties:
      foo: ${poisoned}`
	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	diags := testInvokeDiags(t, tmpl, func(r *Runner) {})
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}

	assert.ElementsMatch(t, diagStrings,
		[]string{
			"<stdin>:5:5: Don't eat the poison",
		})
}

func TestEmptyInterpolate(t *testing.T) {
	t.Parallel()

	text := `
name: test-empty
runtime: yaml
variables:
  empty: ${}
`
	_, diags, err := LoadYAMLBytes("<stdin>", []byte(strings.TrimSpace(text)))
	require.NoError(t, err)
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}

	assert.ElementsMatch(t, diagStrings,
		[]string{
			"<stdin>:4:10: Property access expressions cannot be empty",
		})
}

func TestReadResource(t *testing.T) {
	t.Parallel()
	text := `
name: consumer
runtime: yaml
resources:
  bucket:
    type: test:read:Resource
    get:
      id: ${id}
      state:
        foo: bar
variables:
  id: bucket-123456
  isRight: ${bucket.tags["isRight"]}
`
	templ := yamlTemplate(t, text)
	var wasRun bool
	diags := testInvokeDiags(t, templ, func(r *Runner) {
		r.variables["isRight"].(pulumi.AnyOutput).ApplyT(func(s interface{}) interface{} {
			wasRun = true
			assert.Equal(t, "yes", s)
			return s
		})
	})
	assert.True(t, wasRun)
	assert.Len(t, diags, 0)
}

func TestReadResourceNoState(t *testing.T) {
	t.Parallel()
	text := `
name: consumer
runtime: yaml
resources:
  bucket:
    type: test:read:Resource
    get:
      id: no-state
variables:
  id: bucket-123456
  isRight: ${bucket.tags["isRight"]}
`
	templ := yamlTemplate(t, text)
	var wasRun bool
	diags := testInvokeDiags(t, templ, func(r *Runner) {
		r.variables["isRight"].(pulumi.AnyOutput).ApplyT(func(s interface{}) interface{} {
			wasRun = true
			assert.Equal(t, "yes", s)
			return s
		})
	})
	assert.True(t, wasRun)
	assert.Len(t, diags, 0)
}

func TestReadResourceErrorTyping(t *testing.T) {
	t.Parallel()
	text := `
name: consumer
runtime: yaml
resources:
  bucket:
    type: test:read:Resource
    properties:
      foo: bar
    get:
      state:
        fizz: buzz
`
	templ := yamlTemplate(t, text)
	diags := testTemplateDiags(t, templ, nil)
	assert.Len(t, diags, 2)
	var diagStrings []string
	for _, v := range diags {
		diagStrings = append(diagStrings, diagString(v))
	}
	assert.ElementsMatch(t, diagStrings, []string{
		"<stdin>:5:3: Resource fields properties and get are mutually exclusive",
		"<stdin>:11:9: Property fizz does not exist on 'test:read:Resource'",
	})
}

func TestGetPulumiConfNodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		typ   ctypes.Type
		value interface{}
		err   error
	}{
		{
			input: "foo",
			typ:   ctypes.String,
			value: "foo",
		},
		{
			input: "2.0",
			typ:   ctypes.Number,
			value: 2.0,
		},
		{
			input: "1",
			typ:   ctypes.Int,
			value: int64(1),
		},
		{
			input: "true",
			typ:   ctypes.Boolean,
			value: true,
		},
		{
			input: `["a", "b", "c"]`,
			typ:   ctypes.StringList,
			value: []interface{}{"a", "b", "c"},
		},
		{
			input: `["one", 2]`,
			err:   fmt.Errorf("heterogeneous typed lists are not allowed: found types string and number"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			value, typ, err := getConfigNode(tt.input)
			if tt.err != nil {
				assert.ErrorContains(t, err, tt.err.Error())
			} else {
				assert.Equal(t, tt.typ, typ)
				assert.Equal(t, tt.value, value)
			}
		})
	}
}

// This test checks that resource properties that are unavailable during preview are marked
// unknown.
func TestHandleUnknownPropertiesDuringPreview(t *testing.T) {
	t.Parallel()
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		e := &programEvaluator{
			pulumiCtx: ctx,
			evalContext: &evalContext{
				Runner: &Runner{
					t: &ast.TemplateDecl{},
					resources: map[string]lateboundResource{
						"image": &mockLateboundResource{
							resourceSchema: &schema.Resource{
								InputProperties: []*schema.Property{
									{
										Name: "imageName",
										Type: schema.StringType,
									},
								},
								Properties: []*schema.Property{
									{
										Name: "baseImageName",
										Type: schema.StringType,
									},
								},
							},
						},
					},
				},
			},
		}

		node, diags := ast.ParseExpr(syntax.String("${image.baseImageName}"))
		require.False(t, diags.HasErrors())

		symbolExpr, ok := node.(*ast.SymbolExpr)
		require.True(t, ok)

		result, ok := e.evaluatePropertyAccess(symbolExpr, symbolExpr.Property)
		require.True(t, ok)
		require.False(t, e.sdiags.HasErrors())

		ctx.Export("unexpected-unknown-property", result.(pulumi.AnyOutput))

		return nil
	}, pulumi.WithMocks(testProject, "unknowns", &testMonitor{}), func(ri *pulumi.RunInfo) {
		ri.DryRun = true
	})
	assert.NoError(t, err)
}

type mockLateboundResource struct {
	resourceSchema *schema.Resource
}

var _ lateboundResource = (*mockLateboundResource)(nil)

// GetOutputs returns the resource's outputs.
func (st mockLateboundResource) GetOutputs() pulumi.Output {
	panic("not implemented")
}

// GetOutput returns the named output of the resource.
func (st *mockLateboundResource) GetOutput(k string) pulumi.Output {
	panic("not implemented")
}

func (st *mockLateboundResource) CustomResource() *pulumi.CustomResourceState {
	panic("not implemented")
}

func (st *mockLateboundResource) ProviderResource() *pulumi.ProviderResourceState {
	panic("not implemented")
}

func (st *mockLateboundResource) ElementType() reflect.Type {
	panic("not implemented")
}

func (st *mockLateboundResource) GetRawOutputs() pulumi.Output {
	return pulumi.Any(resource.PropertyMap{})
}

func (st *mockLateboundResource) GetResourceSchema() *schema.Resource {
	return st.resourceSchema
}

// TestResourceMissingType ensures that we fail with an error message when a resource is missing a type.
func TestResourceMissingType(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  my-resource:
    foo: bar
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
		CallF: func(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
			return resource.PropertyMap{}, fmt.Errorf("Unexpected invoke %s", args.Token)
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		return RunTemplate(ctx, template, nil, newMockPackageMap())
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	assert.ErrorContains(t, err, `Required field 'type' is missing on resource "my-resource"`)
}

// This test checks that resource properties that are unavailable during preview are marked unknown.
// Regression test for https://github.com/pulumi/pulumi-yaml/issues/489.
func TestHandleUnknownNestedPropertiesDuringPreview(t *testing.T) {
	t.Parallel()
	// Pretty much a copy of TestHandleUnknownPropertiesDuringPreview but with an index expression
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		e := &programEvaluator{
			pulumiCtx: ctx,
			evalContext: &evalContext{
				Runner: &Runner{
					t: &ast.TemplateDecl{},
					resources: map[string]lateboundResource{
						"image": &mockLateboundResource{
							resourceSchema: &schema.Resource{
								InputProperties: []*schema.Property{
									{
										Name: "imageName",
										Type: schema.StringType,
									},
								},
								Properties: []*schema.Property{
									{
										Name: "baseImageName",
										Type: &schema.ArrayType{ElementType: schema.StringType},
									},
								},
							},
						},
					},
				},
			},
		}

		node, diags := ast.ParseExpr(syntax.String("${image.baseImageName[0]}"))
		require.False(t, diags.HasErrors())

		symbolExpr, ok := node.(*ast.SymbolExpr)
		require.True(t, ok)

		result, ok := e.evaluatePropertyAccess(symbolExpr, symbolExpr.Property)
		require.True(t, ok)
		require.False(t, e.sdiags.HasErrors())

		ctx.Export("unexpected-unknown-property", result.(pulumi.AnyOutput))

		return nil
	}, pulumi.WithMocks(testProject, "unknowns", &testMonitor{}), func(ri *pulumi.RunInfo) {
		ri.DryRun = true
	})
	assert.NoError(t, err)
}

// This test checks that unknown outputs are marked in preview and not during update.
// Regression test for https://github.com/pulumi/pulumi-yaml/issues/492.
func TestUnknownsDuringPreviewNotUpdate(t *testing.T) {
	t.Parallel()
	// Pretty much a copy of TestHandleUnknownPropertiesDuringPreview but with an index expression
	runProgram := func(isPreview bool) error {
		return pulumi.RunErr(func(ctx *pulumi.Context) error {
			e := &programEvaluator{
				pulumiCtx: ctx,
				evalContext: &evalContext{
					Runner: &Runner{
						t: &ast.TemplateDecl{},
						resources: map[string]lateboundResource{
							"image": &mockLateboundResource{
								resourceSchema: &schema.Resource{
									InputProperties: []*schema.Property{
										{
											Name: "imageName",
											Type: schema.StringType,
										},
									},
									Properties: []*schema.Property{
										{
											Name: "baseImageName",
											Type: &schema.ArrayType{ElementType: schema.StringType},
										},
									},
								},
							},
						},
					},
				},
			}

			node, diags := ast.ParseExpr(syntax.String("${image.baseImageName[0]}"))
			require.False(t, diags.HasErrors())

			symbolExpr, ok := node.(*ast.SymbolExpr)
			require.True(t, ok)

			result, ok := e.evaluatePropertyAccess(symbolExpr, symbolExpr.Property)
			require.True(t, ok)
			require.False(t, e.sdiags.HasErrors())

			ctx.Export("unexpected-unknown-property", result.(pulumi.AnyOutput))

			return nil
		}, pulumi.WithMocks(testProject, "unknowns", &testMonitor{}), func(ri *pulumi.RunInfo) {
			ri.DryRun = isPreview
		})
	}
	assert.NoError(t, runProgram(true))
	assert.Error(t, runProgram(false))
}
