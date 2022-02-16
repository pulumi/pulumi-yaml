// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
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
	CallF        func(tok string, args resource.PropertyMap, provider string) (resource.PropertyMap, error)
	NewResourceF func(typeToken, name string, inputs resource.PropertyMap,
		provider, id string) (string, resource.PropertyMap, error)
}

func (m *testMonitor) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	if m.CallF == nil {
		return resource.PropertyMap{}, nil
	}
	return m.CallF(args.Token, args.Args, args.Provider)
}

func (m *testMonitor) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if m.NewResourceF == nil {
		return args.Name, resource.PropertyMap{}, nil
	}
	return m.NewResourceF(args.TypeToken, args.Name, args.Inputs, args.Provider, args.ID)
}

func testTemplateDiags(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) syntax.Diagnostics {
	mocks := &testMonitor{
		NewResourceF: func(typeToken, name string, state resource.PropertyMap,
			provider, id string) (string, resource.PropertyMap, error) {

			switch typeToken {
			case testResourceToken:
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
	return nil
}

func testTemplateSyntaxDiags(t *testing.T, template *ast.TemplateDecl, callback func(*runner)) syntax.Diagnostics {
	// Same mocks as in testTemplateDiags but without assertions, just pure syntax checking.
	mocks := &testMonitor{
		NewResourceF: func(typeToken, name string, state resource.PropertyMap,
			provider, id string) (string, resource.PropertyMap, error) {

			switch typeToken {
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
		Resources: map[string]*Resource{},
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
		out := v.(pulumi.StringOutput).ApplyT(func(x string) (interface{}, error) {
			assert.Equal(t, "a,b,c", x)
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
}

func TestSelect(t *testing.T) {
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
		v, diags := r.evaluateBuiltinSelect(&ast.SelectExpr{
			Index: &ast.GetAttExpr{
				ResourceName: ast.String("resA"),
				PropertyName: ast.String("outNum"),
			},
			Values: ast.List(
				ast.String("first"),
				ast.String("second"),
				ast.String("third"),
			),
		})
		requireNoErrors(t, diags)
		out := pulumi.ToOutput(v).ApplyT(func(x interface{}) (interface{}, error) {
			assert.Equal(t, "second", x.(string))
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
}

func TestSub(t *testing.T) {
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
		v, diags := r.evaluateBuiltinSub(&ast.SubExpr{
			Interpolate: ast.MustInterpolate("Hello ${resA.out} - ${resA}!!"),
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
