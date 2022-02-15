package codegen

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
)

func TestGenerateProgram(t *testing.T) {

	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{}
		for _, tt := range tests {
			switch tt.Directory {
			case "aws-s3-folder", "aws-fargate":
				// Reason: need toJSON function
			case "aws-eks":
				// Reason: missing splat
			case "functions":
				// Reason: missing toBase64
			default:
				l = append(l, tt)
			}
		}
		return l
	}

	test.TestProgramCodegen(t,
		test.ProgramCodegenOptions{
			Language:   "yaml",
			Extension:  "yaml",
			OutputFile: "Main.yaml",
			Check:      nil,
			GenProgram: GenerateProgram,
			TestCases:  filter(test.PulumiPulumiProgramTests),
		},
	)
}
