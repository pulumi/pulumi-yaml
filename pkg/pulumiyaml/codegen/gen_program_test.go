package codegen

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
)

func TestGenerateProgram(t *testing.T) {
	test.TestProgramCodegen(t,
		test.ProgramCodegenOptions{
			Language:   "yaml",
			Extension:  "yaml",
			OutputFile: "Main.yaml",
			Check:      nil,
			GenProgram: GenerateProgram,
			TestCases: []test.ProgramTest{
				{
					Directory:   "azure-sa",
					Description: "Azure SA",
				},
			},
		},
	)
}
