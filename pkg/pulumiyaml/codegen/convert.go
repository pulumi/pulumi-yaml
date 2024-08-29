// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

// A GenerateFunc generates a set of output files from a PCL program. This is used to convert YAML templates to
// higher-level languages using PCL as an intermediate representation.
type GenerateFunc func(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error)

func ConvertTemplateIL(template *ast.TemplateDecl, loader schema.ReferenceLoader) (string, hcl.Diagnostics, error) {
	contract.Assertf(template != nil, "template must not be nil")
	contract.Assertf(loader != nil, "loader must not be nil")

	var diags hcl.Diagnostics

	pkgLoader := pulumiyaml.NewPackageLoaderFromSchemaLoader(loader)
	// nil runner passed in since template is not executed and we can use pkgLoader
	_, tdiags, err := pulumiyaml.PrepareTemplate(template, nil, pkgLoader)
	if err != nil {
		return "", diags, err
	}
	diags = diags.Extend(tdiags.HCL())

	templateBody, tdiags := ImportTemplate(template, pkgLoader)
	diags = diags.Extend(tdiags.HCL())
	if templateBody == nil {
		// This is a irrecoverable error, so we make sure the error field is non-nil
		return "", diags, diags
	}
	programText := fmt.Sprintf("%v", templateBody)

	if programText == "" {
		return "", diags, diags
	}

	return programText, diags, nil
}

func EjectProgram(template *ast.TemplateDecl, loader schema.ReferenceLoader) (*pcl.Program, hcl.Diagnostics, error) {
	programText, yamlDiags, err := ConvertTemplateIL(template, loader)
	if err != nil || programText == "" {
		return nil, yamlDiags, err
	}

	parser := hclsyntax.NewParser()
	if err := parser.ParseFile(strings.NewReader(programText), "program.pp"); err != nil {
		return nil, yamlDiags, err
	}
	diags := parser.Diagnostics
	if diags.HasErrors() {
		return nil, append(yamlDiags, diags...), diags
	}

	bindOpts := []pcl.BindOption{
		pcl.SkipResourceTypechecking,
		pcl.AllowMissingProperties,
		pcl.AllowMissingVariables,
	}
	bindOpts = append(bindOpts, pcl.Loader(loader))
	program, pdiags, err := pcl.BindProgram(parser.Files, bindOpts...)
	diags = diags.Extend(pdiags)
	if err != nil {
		return nil, append(yamlDiags, diags...), err
	}
	if pdiags.HasErrors() || program == nil {
		return nil, append(yamlDiags, diags...), fmt.Errorf("internal error: %w", pdiags)
	}

	return program, append(yamlDiags, diags...), nil
}

// ConvertTemplate converts a Pulumi YAML template to a target language using PCL as an intermediate representation.
//
// loader is the schema.Loader used when binding the the PCL program. If `nil`, a `schema.Loader` will be created from `newPluginHost()`.
func ConvertTemplate(template *ast.TemplateDecl, generate GenerateFunc, loader schema.ReferenceLoader) (map[string][]byte, hcl.Diagnostics, error) {
	program, diags, err := EjectProgram(template, loader)
	if err != nil {
		return nil, diags, err
	}
	if diags.HasErrors() {
		return nil, diags, fmt.Errorf("internal error: %w", diags)
	}

	files, gdiags, err := generate(program)
	diags = diags.Extend(gdiags)
	return files, diags, err
}
