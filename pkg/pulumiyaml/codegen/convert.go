// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	pkgWorkspace "github.com/pulumi/pulumi/pkg/v3/workspace"
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
	pluginCtx, err := plugin.NewContext(ctx, sink, sink, nil, nil, cwd, nil, true, nil,
		schema.NewLoaderServerFromHost, pkgWorkspace.EnsureLanguageInstalled)
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

// InputHints returns the input property types declared by the resource, function, or provider
// identified by token in pkg. The token may be a resource token (e.g. "aws:s3/bucket:Bucket"), a
// function token (e.g. "aws:s3:getBucket"), or a provider token of the form "pulumi:providers:pkg".
func InputHints(pkg pulumiyaml.Package, token string) (map[string]schema.Type, error) {
	hints := map[string]schema.Type{}
	if resTok, rerr := pkg.ResolveResource(token); rerr == nil {
		if hint := pkg.ResourceTypeHint(resTok); hint != nil && hint.Resource != nil {
			for _, p := range hint.Resource.InputProperties {
				hints[p.Name] = p.Type
			}
		}
		return hints, nil
	}
	if fnTok, ferr := pkg.ResolveFunction(token); ferr == nil {
		if hint := pkg.FunctionTypeHint(fnTok); hint != nil && hint.Inputs != nil {
			for _, p := range hint.Inputs.Properties {
				hints[p.Name] = p.Type
			}
		}
		return hints, nil
	}
	return nil, fmt.Errorf("token %q is not a resource, function, or provider of package %q",
		token, pkg.Name())
}

// newSnippetImporter constructs an importer with empty bookkeeping maps. It's the shared
// entrypoint for snippet-level conversions (bodies and individual attributes), which don't have a
// surrounding template to register variables/resources/outputs against.
func newSnippetImporter(loader pulumiyaml.PackageLoader) *importer {
	return &importer{
		loader:             loader,
		configuration:      map[string]*model.Variable{},
		variables:          map[string]*model.Variable{},
		stackReferences:    map[string]*model.Variable{},
		resources:          map[string]*model.Variable{},
		outputs:            map[string]*model.Variable{},
		packageDescriptors: map[tokens.Package]*schema.PackageDescriptor{},
		snippet:            true,
	}
}

// parseSnippetExpr decodes a single YAML expression source into an ast.Expr. Unlike DecodeYAML it
// doesn't force the top-level node to be an object, so individual attribute values that resolve to
// scalars, lists, or template expressions all round-trip. Returned diagnostics describe any
// parse/decode failures and may be present even when err is nil.
func parseSnippetExpr(filename string, source []byte) (ast.Expr, syntax.Diagnostics) {
	var diags syntax.Diagnostics
	var yamlNode yaml.Node
	if err := yaml.Unmarshal(source, &yamlNode); err != nil {
		diags.Extend(syntax.Error(nil, err.Error(), ""))
		return nil, diags
	}
	// yaml.Unmarshal wraps the parsed value in a document node; unwrap so UnmarshalYAML sees the
	// real content.
	if yamlNode.Kind == yaml.DocumentNode {
		if len(yamlNode.Content) != 1 {
			return nil, diags
		}
		yamlNode = *yamlNode.Content[0]
	}
	syn, sdiags := encoding.UnmarshalYAML(filename, &yamlNode, pulumiyaml.TagDecoder)
	diags.Extend(sdiags...)
	if sdiags.HasErrors() {
		return nil, diags
	}
	expr, ediags := ast.ParseExpr(syn)
	diags.Extend(ediags...)
	if ediags.HasErrors() {
		return nil, diags
	}
	return expr, diags
}

// ImportSnippet converts a YAML mapping describing the inputs to the resource, function, or
// provider identified by token into a PCL body of attribute = value assignments. Field types are
// looked up against pkg so that nested objects, lists, and quoted vs. plain strings render
// correctly. loader is used to resolve packages referenced by nested expressions (e.g. fn::invoke).
func ImportSnippet(
	ctx context.Context, token, filename string, source []byte,
	pkg pulumiyaml.Package, loader pulumiyaml.PackageLoader,
) (*model.Body, syntax.Diagnostics, error) {
	var diags syntax.Diagnostics

	hints, err := InputHints(pkg, token)
	if err != nil {
		return nil, diags, err
	}

	expr, pdiags := parseSnippetExpr(filename, source)
	diags.Extend(pdiags...)
	if pdiags.HasErrors() {
		return nil, diags, nil
	}
	obj, ok := expr.(*ast.ObjectExpr)
	if !ok {
		return nil, diags, fmt.Errorf("snippet %q must be a YAML mapping", filename)
	}

	imp := newSnippetImporter(loader)

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

// ImportSnippetAttributes converts a map of per-attribute YAML expression sources into PCL
// expression literals keyed by attribute name. Each value is parsed as an independent YAML
// expression and run through the same importer pipeline as ImportSnippet, so template
// interpolations (${...}), fn::invoke calls, and structured values all convert the same way they
// would inside a snippet body.
func ImportSnippetAttributes(
	ctx context.Context, attrs map[string]string,
	token string, pkg pulumiyaml.Package, loader pulumiyaml.PackageLoader,
) (map[string]string, syntax.Diagnostics, error) {
	var diags syntax.Diagnostics
	if len(attrs) == 0 {
		return nil, diags, nil
	}

	hints, err := InputHints(pkg, token)
	if err != nil {
		return nil, diags, err
	}

	imp := newSnippetImporter(loader)
	var f formatter

	out := make(map[string]string, len(attrs))
	for name, raw := range attrs {
		expr, pdiags := parseSnippetExpr(name, []byte(raw))
		diags.Extend(pdiags...)
		if expr == nil {
			continue
		}
		v, vdiags := imp.importExpr(expr, hints[name])
		diags.Extend(vdiags...)
		out[name] = fmt.Sprintf("%v", f.formatExpression(v))
	}
	return out, diags, nil
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
