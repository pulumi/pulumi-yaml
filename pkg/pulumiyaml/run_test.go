// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	b64 "encoding/base64"
	"fmt"
	"strings"
	"testing"

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
	resourceTypeHint func(typeName string) InputTypeHint
	functionTypeHint func(typeName string) InputTypeHint
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

func (m MockPackage) ResourceTypeHint(typeName ResourceTypeToken) InputTypeHint {
	return m.resourceTypeHint(typeName.String())
}

func (m MockPackage) FunctionTypeHint(typeName FunctionTypeToken) InputTypeHint {
	return m.functionTypeHint(typeName.String())
}

func (m MockPackage) Name() string {
	return "test"
}

type mockInputTypeHint []string

func (m mockInputTypeHint) InputProperties() FieldsTypeHint {
	return m.Fields()
}

func (m mockInputTypeHint) Fields() FieldsTypeHint {
	o := FieldsTypeHint{}
	for _, f := range m {
		o[f] = nil
	}
	return o
}
func (m mockInputTypeHint) Element() TypeHint { return nil }

func newMockPackageMap() PackageLoader {
	return MockPackageLoader{
		packages: map[string]Package{
			"test": MockPackage{
				resourceTypeHint: func(typeName string) InputTypeHint {
					switch typeName {
					case testResourceToken:
						return mockInputTypeHint{"foo"}
					case testComponentToken:
						return mockInputTypeHint{"foo"}
					default:
						return mockInputTypeHint{}
					}
				},
				functionTypeHint: func(typeName string) InputTypeHint {
					switch typeName {
					case "test:fn":
						return mockInputTypeHint{"yesArg", "someSuchArg"}
					default:
						return mockInputTypeHint{}
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
		diags := TypeCheck(runner)
		if diags.HasErrors() {
			return diags
		}
		err := runner.Evaluate()
		if err != nil {
			return err
		}
		if callback != nil {
			ctx := runner.newContext(nil)
			callback(ctx)
		}
		return nil
	}, pulumi.WithMocks("foo", "dev", mocks))
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
    component: true
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
	assert.Equal(t, "<stdin>:9:8: resource or variable named res-b could not be found", diagString(diags[0]))
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
			"component": true,
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
	assert.Equal(t, "<stdin>:13:10: resource or variable named res-b could not be found", diagString(diags[0]))
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
      foo: does exist
      bar: does not exist
`
	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *evalContext) {})
	require.True(t, diags.HasErrors())
	assert.Len(t, diags, 2)
	assert.Equal(t, "<stdin>:17:7: Property 'bar' does not exist on Resource 'test:resource:type'",
		diagString(diags[0]))
	assert.Equal(t, "<stdin>:10:9: noArg does not exist on Invoke test:fn", diagString(diags[1]))

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
