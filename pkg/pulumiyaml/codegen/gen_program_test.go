// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"os"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/stretchr/testify/assert"
)

func TestGenerateProgram(t *testing.T) {

	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{}
		for _, tt := range tests {
			switch tt.Directory {
			case "aws-resource-options":
				// We generate incorrect code right now
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
		_, diags, err := pulumiyaml.LoadYAMLBytes(output, file)
		assert.NoError(t, err)
		assert.Falsef(t, diags.HasErrors(), "%s", diags.Error())
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
