// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"os"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

func TestGenerateProgram(t *testing.T) {

	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{}
		for _, tt := range tests {
			switch tt.Directory {
			case "azure-sa", "aws-webserver", "aws-optionals":
				// Waiting on variables merging
			case "aws-s3-folder", "aws-fargate":
				// Reason: need toJSON function
			case "aws-eks":
				// Reason: missing splat
			case "functions":
				// Reason: missing toBase64
			case "output-funcs-aws":
				// Calls invoke without assigning the result
				// Right now this fails a contract. For the future, we can either:
				// 1. Construct an arbitrary return value
				// 2. Not generate the invoke at all
			default:
				l = append(l, tt)
			}
		}
		return l
	}

	check := func(t *testing.T, output string, _ codegen.StringSet) {
		file, err := os.ReadFile(output)
		assert.NoError(t, err)
		templateDecl, diags, err := pulumiyaml.LoadYAMLBytes(output, file)
		assert.NoError(t, err)
		assert.Falsef(t, diags.HasErrors(), "%s", diags.Error())
		err = pulumi.RunErr(func(ctx *pulumi.Context) error {
			return pulumiyaml.RunTemplate(ctx, templateDecl)
		}, pulumi.WithMocks("test", "gen", &testMonitor{}))
		assert.NoError(t, err)
	}

	test.TestProgramCodegen(t,
		test.ProgramCodegenOptions{
			Language:   "yaml",
			Extension:  "yaml",
			OutputFile: "Main.yaml",
			Check:      check,
			GenProgram: GenerateProgram,
			TestCases:  filter(test.PulumiPulumiProgramTests),
		},
	)
}

type testMonitor struct{}

func (m *testMonitor) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func (m *testMonitor) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	switch args.Name {
	case "bucket":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"loggings": []interface{}{
				map[string]string{
					"targetBucket": "foo",
				},
			},
		}), nil
	case "logs":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"bucket": "foo",
		}), nil
	}

	return args.Name, resource.PropertyMap{}, nil
}
