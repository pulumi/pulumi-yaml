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

const (
	idAttr = "anID"
)

type interceptingLog struct {
	debugMessages []string
	infoMessages  []string
	warnMessages  []string
	errorMessages []string
}

func (l *interceptingLog) Debug(msg string, args *pulumi.LogArgs) error {
	l.debugMessages = append(l.debugMessages, msg)
	return nil
}
func (l *interceptingLog) Info(msg string, args *pulumi.LogArgs) error {
	l.infoMessages = append(l.infoMessages, msg)
	return nil
}
func (l *interceptingLog) Warn(msg string, args *pulumi.LogArgs) error {
	l.warnMessages = append(l.warnMessages, msg)
	return nil
}
func (l *interceptingLog) Error(msg string, args *pulumi.LogArgs) error {
	l.errorMessages = append(l.errorMessages, msg)
	return nil
}

// Test that errors within applies propagate to Pulumi's error logging
func TestAsyncDiagsOptions(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
  res-b:
    type: test:resource:type
    properties:
      foo: ${res-a.bar[1]}
`
	template := yamlTemplate(t, strings.TrimSpace(text))

	calls := 0

	mocks := &testMonitor{
		NewResourceF: func(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
			calls = calls + 1
			switch args.TypeToken {
			case testResourceToken:
				return idAttr, resource.PropertyMap{
					"bar": resource.NewArrayProperty([]resource.PropertyValue{
						resource.NewNumberProperty(42),
					}),
				}, nil
			}
			return "", resource.PropertyMap{}, fmt.Errorf("Unexpected resource type %s", args.TypeToken)
		},
	}

	log := &interceptingLog{}

	var hoistedRunner *runner
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		ctx.Log = log
		r := newRunner(ctx, template, newMockPackageMap())
		hoistedRunner = r
		diags := r.Evaluate()
		// 1. This test demonstrates that the synchronous output of evaluate is nil
		// as the invalid expression is inside an apply
		assert.False(t, diags.HasErrors())
		return diags
	}, pulumi.WithMocks("projectFoo", "stackDev", mocks))
	// 2. The internal error in an apply is bubbled up by the engine via RPC:
	assert.Error(t, err)
	assert.Equal(t, "waiting for RPCs: marshaling properties: awaiting input property foo: runtime error", err.Error())

	// 3. The runner on the YAML side processed the inner error:
	assert.True(t, hoistedRunner.sdiags.HasErrors())
	assert.Equal(t, "<stdin>:9:12: list index 1 out-of-bounds for list of length 1", diagString(hoistedRunner.sdiags.diags[0]))

	// 4. We have rich logs sent to Pulumi:
	richError := `list index 1 out-of-bounds for list of length 1

  on <stdin> line 9:
   1: name: test-yaml

`

	assert.Equal(t, richError, log.errorMessages[0])
}
