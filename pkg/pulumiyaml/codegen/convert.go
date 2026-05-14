// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax/encoding"
)

// A GenerateFunc generates a set of output files from a PCL program. This is used to convert YAML templates to
// higher-level languages using PCL as an intermediate representation.
type GenerateFunc func(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error)

func newPluginHost() (plugin.Host, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	sink := diag.DefaultSink(os.Stderr, os.Stderr, diag.FormatOptions{
		Color: cmdutil.GetGlobalColorization(),
	})
	ctx := context.Background()
	pluginCtx, err := plugin.NewContext(ctx, sink, sink, nil, nil, cwd, nil, true, nil, nil)
	if err != nil {
		return nil, err
	}
	return pluginCtx.Host, nil
}

func ConvertTemplateIL(template *ast.TemplateDecl, loader schema.ReferenceLoader) (string, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics

	if loader == nil {
		host, err := newPluginHost()
		if err != nil {
			return "", diags, err
		}
		loader = schema.NewPluginLoader(host)
		defer contract.IgnoreClose(host)
	}

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

	return programText, diags, nil
}

// ImportSnippet converts a YAML mapping describing the inputs to the resource, function, or
// provider identified by token into a PCL body of attribute = value assignments. Field types
// are looked up against the token's schema so that nested objects, lists, and quoted vs. plain
// strings render correctly. The token may be a resource token (e.g. "aws:s3/bucket:Bucket"), a
// function token (e.g. "aws:s3:getBucket"), or a provider token of the form "pulumi:providers:pkg".
func ImportSnippet(
	ctx context.Context, token, filename string, source []byte, loader pulumiyaml.PackageLoader,
) (*model.Body, syntax.Diagnostics, error) {
	var diags syntax.Diagnostics

	pkgName := pulumiyaml.ResolvePkgName(token)
	pkg, err := loader.LoadPackage(ctx, &schema.PackageDescriptor{Name: pkgName})
	if err != nil {
		return nil, diags, fmt.Errorf("loading package %q: %w", pkgName, err)
	}

	// Look up the input properties for the token. ResolveResource handles both regular
	// resource tokens and "pulumi:providers:pkg" provider tokens (via resolveProvider).
	hints := map[string]schema.Type{}
	var resolveErr error
	if resTok, rerr := pkg.ResolveResource(token); rerr == nil {
		if hint := pkg.ResourceTypeHint(resTok); hint != nil && hint.Resource != nil {
			for _, p := range hint.Resource.InputProperties {
				hints[p.Name] = p.Type
			}
		}
	} else if fnTok, ferr := pkg.ResolveFunction(token); ferr == nil {
		if hint := pkg.FunctionTypeHint(fnTok); hint != nil && hint.Inputs != nil {
			for _, p := range hint.Inputs.Properties {
				hints[p.Name] = p.Type
			}
		}
	} else {
		resolveErr = fmt.Errorf("token %q is not a resource, function, or provider of package %q: %v; %v",
			token, pkgName, rerr, ferr)
	}
	if resolveErr != nil {
		return nil, diags, resolveErr
	}

	syn, sdiags := encoding.DecodeYAML(filename, yaml.NewDecoder(bytes.NewReader(source)), pulumiyaml.TagDecoder)
	diags.Extend(sdiags...)
	if sdiags.HasErrors() {
		return nil, diags, nil
	}
	expr, ediags := ast.ParseExpr(syn)
	diags.Extend(ediags...)
	if ediags.HasErrors() {
		return nil, diags, nil
	}
	obj, ok := expr.(*ast.ObjectExpr)
	if !ok {
		return nil, diags, fmt.Errorf("snippet %q must be a YAML mapping", filename)
	}

	imp := &importer{
		loader:             loader,
		configuration:      map[string]*model.Variable{},
		variables:          map[string]*model.Variable{},
		stackReferences:    map[string]*model.Variable{},
		resources:          map[string]*model.Variable{},
		outputs:            map[string]*model.Variable{},
		packageDescriptors: map[tokens.Package]*schema.PackageDescriptor{},
	}

	items := make([]model.BodyItem, 0, len(obj.Entries))
	for _, entry := range obj.Entries {
		key, ok := entry.Key.(*ast.StringExpr)
		if !ok {
			diags.Extend(ast.ExprError(entry.Key, "snippet keys must be string literals", ""))
			continue
		}
		v, vdiags := imp.importExpr(entry.Value, hints[key.Value])
		diags.Extend(vdiags...)
		items = append(items, &model.Attribute{Name: key.Value, Value: v})
	}
	body := &model.Body{Items: items}
	formatBody(body)
	return body, diags, nil
}

func EjectProgram(template *ast.TemplateDecl, loader schema.ReferenceLoader) (*pcl.Program, hcl.Diagnostics, error) {
	programText, yamlDiags, err := ConvertTemplateIL(template, loader)
	if err != nil {
		return nil, yamlDiags, err
	}

	parser := hclsyntax.NewParser()
	if programText != "" {
		if err := parser.ParseFile(strings.NewReader(programText), "program.pp"); err != nil {
			return nil, yamlDiags, err
		}
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
