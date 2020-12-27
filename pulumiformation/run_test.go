package pulumiformation

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v2/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type testMonitor struct {
	CallF        func(tok string, args resource.PropertyMap, provider string) (resource.PropertyMap, error)
	NewResourceF func(typeToken, name string, inputs resource.PropertyMap,
		provider, id string) (string, resource.PropertyMap, error)
}

func (m *testMonitor) Call(tok string, args resource.PropertyMap, provider string) (resource.PropertyMap, error) {
	if m.CallF == nil {
		return resource.PropertyMap{}, nil
	}
	return m.CallF(tok, args, provider)
}

func (m *testMonitor) NewResource(typeToken, name string, inputs resource.PropertyMap,
	provider, id string) (string, resource.PropertyMap, error) {

	if m.NewResourceF == nil {
		return name, resource.PropertyMap{}, nil
	}
	return m.NewResourceF(typeToken, name, inputs, provider, id)
}

func testTemplate(t *testing.T, template Template, callback func(*runner)) {
	mocks := &testMonitor{
		NewResourceF: func(typeToken, name string, state resource.PropertyMap,
			provider, id string) (string, resource.PropertyMap, error) {

			assert.Equal(t, "test:resource:type", typeToken)
			assert.Equal(t, "resA", name)
			assert.True(t, state.DeepEquals(resource.NewPropertyMapFromMap(map[string]interface{}{
				"foo": "oof",
			})))
			assert.Equal(t, "", provider)
			assert.Equal(t, "", id)

			return "someID", resource.PropertyMap{
				"foo": resource.NewStringProperty("qux"),
				"out": resource.NewStringProperty("tuo"),
			}, nil
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		runner := newRunner(ctx, template)
		err := runner.Evaluate()
		if err != nil {
			return err
		}
		callback(runner)
		return nil
	}, pulumi.WithMocks("foo", "dev", mocks))
	assert.NoError(t, err)
}

func TestJoin(t *testing.T) {
	tmpl := Template{
		Resources: map[string]*Resource{},
	}
	testTemplate(t, tmpl, func(r *runner) {
		v, err := r.evaluateBuiltinJoin(&Join{
			Delimiter: &Value{Val: pulumi.String(",").ToStringOutput()},
			Values: &Array{
				Elems: []Expr{
					&Value{Val: "a"},
					&Value{Val: pulumi.String("b").ToStringOutput()},
					&Value{Val: "c"},
				},
			},
		})
		assert.NoError(t, err)
		out := v.(pulumi.StringOutput).ApplyT(func(x string) (interface{}, error) {
			assert.Equal(t, "a,b,c", x)
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
}

func TestSub(t *testing.T) {
	tmpl := Template{
		Resources: map[string]*Resource{
			"resA": {
				Type: "test:resource:type",
				Properties: map[string]interface{}{
					"foo": "oof",
				},
			},
		},
	}
	testTemplate(t, tmpl, func(r *runner) {
		v, err := r.evaluateBuiltinSub(&Sub{
			String: "Hello ${resA.out} - ${resA} - ${someName}!!",
			Substitutions: &Object{
				Elems: map[string]Expr{
					"someName": &Value{Val: "someVal"},
				},
			},
		})
		assert.NoError(t, err)
		out := v.(pulumi.StringOutput).ApplyT(func(x string) (interface{}, error) {
			assert.Equal(t, "Hello tuo - someID - someVal!!", x)
			return nil, nil
		})
		r.ctx.Export("out", out)
	})
}
