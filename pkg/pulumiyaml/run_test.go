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

func testTemplateDiags(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) syntax.Diagnostics {
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

func testTemplate(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) {
	diags := testTemplateDiags(t, template, callback)
	requireNoErrors(t, diags)
}

func TestYAML(t *testing.T) {
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
  foo: !GetAtt res-a.foo
  bar: !Ref res-a
`
	tmpl := yamlTemplate(t, text)
	testTemplate(t, tmpl, func(r *runner) {})
}

func TestAssetOrArchive(t *testing.T) {
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
	testTemplate(t, tmpl, func(r *runner) {
		dir, ok := r.variables["dir"]
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
	const text = `name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    properties:
      foo: oof
outputs:
  out: !Ref res-b
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *runner) {})
	require.True(t, diags.HasErrors())
	require.Len(t, diags, 1)
	assert.Equal(t, "<stdin>:9:8: resource Ref named res-b could not be found", diagString(diags[0]))
}

func TestDuplicateKeyDiags(t *testing.T) {
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
	diags := testTemplateDiags(t, tmpl, func(r *runner) {})
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
	diags := testTemplateDiags(t, tmpl, func(r *runner) {})
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
	diags := testTemplateDiags(t, tmpl, func(r *runner) {})
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
		"foo": {
			"Fn::GetAtt": [
				"res-a",
				"bar"
			]
		},
		"bar": {
			"Ref": "res-a"
		}
	}
}`

	tmpl := yamlTemplate(t, text)
	testTemplate(t, tmpl, func(r *runner) {})
}

func TestJSONDiags(t *testing.T) {
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
		"foo": {
			"Ref": "res-b"
		}
	}
}
`

	tmpl := yamlTemplate(t, text)
	diags := testTemplateDiags(t, tmpl, func(r *runner) {})
	require.True(t, diags.HasErrors())
	require.Len(t, diags, 1)
	assert.Equal(t, "<stdin>:13:10: resource Ref named res-b could not be found", diagString(diags[0]))
}

func TestPropertyAccess(t *testing.T) {
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
	testTemplate(t, tmpl, func(r *runner) {
		x, diags := ast.Interpolate("${resA.outList[0].value}")
		requireNoErrors(t, diags)

		v, diags := r.evaluatePropertyAccess(x, x.Parts[0].Value, nil)
		requireNoErrors(t, diags)
		r.ctx.Export("out", pulumi.Any(v))
	})
}

func TestJoin(t *testing.T) {
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
	testTemplate(t, tmpl, func(r *runner) {
		v, diags := r.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: ast.String(","),
			Values: ast.List(
				ast.String("a"),
				ast.String("b"),
				ast.String("c"),
			),
		})
		requireNoErrors(t, diags)
		assert.Equal(t, "a,b,c", v)

		x, diags := ast.Interpolate("${resA.out}")
		requireNoErrors(t, diags)

		v, diags = r.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: x,
			Values: ast.List(
				ast.String("["),
				ast.String("]"),
			),
		})
		requireNoErrors(t, diags)
		out := v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "[tuo]", x)
			return nil, nil
		})
		r.ctx.Export("out", out)

		v, diags = r.evaluateBuiltinJoin(&ast.JoinExpr{
			Delimiter: ast.String(","),
			Values:    ast.List(x, x),
		})
		requireNoErrors(t, diags)
		out = v.(pulumi.Output).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "tuo,tuo", x)
			return nil, nil
		})
		r.ctx.Export("out2", out)
	})
}

func TestSplit(t *testing.T) {
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
	for _, tt := range tests {
		t.Run(strings.Join(tt.expected, ","), func(t *testing.T) {
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
			testTemplate(t, tmpl, func(r *runner) {
				v, diags := r.evaluateBuiltinSplit(tt.input)
				requireNoErrors(t, diags)
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

func TestToJSON(t *testing.T) {
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
		t.Run(tt.expected, func(t *testing.T) {
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
			testTemplate(t, tmpl, func(r *runner) {
				v, diags := r.evaluateBuiltinToJSON(tt.input)
				requireNoErrors(t, diags)
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
				Values: &ast.GetAttExpr{
					ResourceName: ast.String("resA"),
					PropertyName: ast.String("outList"),
				},
			},
			expected: map[string]interface{}{"value": 42.0},
			isOutput: true,
		},
		{
			input: &ast.SelectExpr{
				Index: &ast.GetAttExpr{
					ResourceName: ast.String("resA"),
					PropertyName: ast.String("outNum"),
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
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
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
			testTemplate(t, tmpl, func(r *runner) {
				v, diags := r.evaluateBuiltinSelect(tt.input)
				if tt.isError {
					assert.True(t, diags.HasErrors())
					assert.Nil(t, v)
					return
				}

				requireNoErrors(t, diags)
				if tt.isOutput {
					out := v.(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
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

func TestToBase64(t *testing.T) {
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
		t.Run(tt.expected, func(t *testing.T) {
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
			testTemplate(t, tmpl, func(r *runner) {
				v, diags := r.evaluateBuiltinToBase64(tt.input)
				requireNoErrors(t, diags)
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
	testTemplate(t, tmpl, func(r *runner) {
		v, diags := r.evaluateBuiltinSub(&ast.SubExpr{
			Interpolate: ast.MustInterpolate("Hello ${foo}!"),
		})
		requireNoErrors(t, diags)
		assert.Equal(t, "Hello oof!", v)

		v, diags = r.evaluateBuiltinSub(&ast.SubExpr{
			Interpolate: ast.MustInterpolate("Hello ${resA.out} - ${resA.id}!!"),
		})
		requireNoErrors(t, diags)
		out := v.(pulumi.AnyOutput).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "Hello tuo - someID!!", x)
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
}

func TestRef(t *testing.T) {
	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"resA": {
				Type: testResourceToken,
				Properties: map[string]interface{}{
					"foo": "oof",
				},
			},
			"compA": {
				Type: testComponentToken,
				Properties: map[string]interface{}{
					"foo": "oof",
				},
			},
		},
	})
	testTemplate(t, tmpl, func(r *runner) {
		{
			v, diags := r.evaluateBuiltinRef(ast.Ref("resA"))
			requireNoErrors(t, diags)
			out := v.(pulumi.StringOutput).ApplyT(func(x string) (interface{}, error) {
				assert.Equal(t, "someID", x)
				return nil, nil
			})
			r.ctx.Export("out", out)
		}
		{
			v, diags := r.evaluateBuiltinRef(ast.Ref("compA"))
			requireNoErrors(t, diags)
			out := v.(pulumi.StringOutput).ApplyT(func(x string) (interface{}, error) {
				assert.Equal(t, "", x)
				return nil, nil
			})
			r.ctx.Export("out", out)
		}
	})
}
