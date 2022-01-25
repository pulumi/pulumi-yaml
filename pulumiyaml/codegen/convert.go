package codegen

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"

	"github.com/pulumi/pulumi-yaml/pulumiyaml/ast"
)

// A GenerateFunc generates a set of output files from a PCL program. This is used to convert YAML templates to
// higher-level languages using PCL as an intermediate representation.
type GenerateFunc func(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error)

// ConvertTemplate converts a Pulumi YAML template to a target language using PCL as an intermediate representation.
func ConvertTemplate(template *ast.TemplateDecl, generate GenerateFunc) (map[string][]byte, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics

	templateBody, tdiags := ImportTemplate(template)
	diags = diags.Extend(hcl.Diagnostics(tdiags))
	if diags.HasErrors() {
		return nil, diags, nil
	}
	programText := fmt.Sprintf("%v", templateBody)

	parser := hclsyntax.NewParser()
	if err := parser.ParseFile(strings.NewReader(programText), "prorgram.pp"); err != nil {
		return nil, diags, err
	}
	diags.Extend(parser.Diagnostics)
	if diags.HasErrors() {
		return nil, diags, nil
	}

	program, pdiags, err := pcl.BindProgram(parser.Files, pcl.SkipResourceTypechecking, pcl.AllowMissingProperties,
		pcl.AllowMissingVariables)
	if err != nil {
		return nil, diags, err
	}
	if pdiags.HasErrors() {
		return nil, diags, fmt.Errorf("internal error: %w", pdiags)
	}

	ioutil.WriteFile("program.pp", []byte(programText), 0600)

	files, gdiags, err := generate(program)
	diags = diags.Extend(gdiags)
	return files, diags, err
}
