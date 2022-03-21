// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

// A GenerateFunc generates a set of output files from a PCL program. This is used to convert YAML templates to
// higher-level languages using PCL as an intermediate representation.
type GenerateFunc func(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error)

func newPluginHost() (plugin.Host, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	sink := diag.DefaultSink(os.Stderr, os.Stderr, diag.FormatOptions{})
	pluginCtx, err := plugin.NewContext(sink, sink, nil, nil, cwd, nil, true, nil)
	if err != nil {
		return nil, err
	}
	return pluginCtx.Host, nil
}

// ConvertTemplate converts a Pulumi YAML template to a target language using PCL as an intermediate representation.
//
// host is the plugin.Host used when binding the the PCL program. If nil, the default plugin.Host is used.
func ConvertTemplate(template *ast.TemplateDecl, generate GenerateFunc, loader schema.Loader) (map[string][]byte, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics

	if loader == nil {
		host, err := newPluginHost()
		if err != nil {
			return nil, diags, err
		}
		loader = schema.NewPluginLoader(host)
	}

	templateBody, tdiags := ImportTemplate(template, pulumiyaml.NewPackageLoaderFromSchemaLoader(loader))
	diags = diags.Extend(hcl.Diagnostics(tdiags))
	if diags.HasErrors() {
		return nil, diags, nil
	}
	programText := fmt.Sprintf("%v", templateBody)

	parser := hclsyntax.NewParser()
	if err := parser.ParseFile(strings.NewReader(programText), "program.pp"); err != nil {
		return nil, diags, err
	}
	diags = diags.Extend(parser.Diagnostics)
	if diags.HasErrors() {
		return nil, diags, nil
	}
	bindOpts := []pcl.BindOption{
		pcl.SkipResourceTypechecking,
		pcl.AllowMissingProperties,
		pcl.AllowMissingVariables,
	}
	bindOpts = append(bindOpts, pcl.Loader(loader))
	program, pdiags, err := pcl.BindProgram(parser.Files, bindOpts...)
	if err != nil {
		return nil, diags, err
	}
	if pdiags.HasErrors() {
		return nil, diags, fmt.Errorf("internal error: %w", pdiags)
	}

	files, gdiags, err := generate(program)
	diags = diags.Extend(gdiags)
	return files, diags, err
}
