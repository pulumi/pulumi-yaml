// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/iancoleman/strcase"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
)

type CanonicalTypeToken string

// Package is our external facing term, e.g.: a provider package in the registry. Packages are
// delivered via plugins, and this interface provides enough surface area to get information about
// resources in a package.
type Package interface {
	ResolveResource(typeName string) (CanonicalTypeToken, error)
	ResolveFunction(typeName string) (CanonicalTypeToken, error)
	IsComponent(typeName CanonicalTypeToken) (bool, error)
}

type PackageMap map[string]Package

// Plugin is metadata containing a package name, possibly empty version and download URL. Used to
// inform the engine of the required plugins at the beginning of program execution.
type Plugin struct {
	Package           string
	Version           string
	PluginDownloadURL string
}

type pluginEntry struct {
	version           string
	pluginDownloadURL string
}

// GetReferencedPlugins returns the packages and (if provided) versions for each referenced provider
// used in the program.
func GetReferencedPlugins(tmpl *ast.TemplateDecl) ([]Plugin, syntax.Diagnostics) {
	pluginMap := map[string]*pluginEntry{}

	var diags syntax.Diagnostics

	for _, kvp := range tmpl.Resources.Entries {
		res := kvp.Value
		version := res.Options.Version.GetValue()
		pluginDownloadURL := res.Options.PluginDownloadURL.GetValue()

		pkg := resolvePkgName(res.Type.Value)
		if entry, found := pluginMap[pkg]; found {
			if version != "" && entry.version != version {
				if entry.version == "" {
					entry.version = version
				} else {
					diags.Extend(ast.ExprError(res.Options.Version, fmt.Sprintf("Provider %v already declared with a conflicting version: %v", pkg, entry.version), ""))
				}
			}
			if pluginDownloadURL != "" && entry.pluginDownloadURL != pluginDownloadURL {
				if entry.pluginDownloadURL == "" {
					entry.pluginDownloadURL = pluginDownloadURL
				} else {
					diags.Extend(ast.ExprError(res.Options.PluginDownloadURL, fmt.Sprintf("Provider %v already declared with a conflicting plugin download URL: %v", pkg, entry.pluginDownloadURL), ""))
				}
			}
		} else {
			pluginMap[pkg] = &pluginEntry{
				version:           version,
				pluginDownloadURL: pluginDownloadURL,
			}
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}

	var plugins []Plugin
	for pkg, meta := range pluginMap {
		plugins = append(plugins, Plugin{
			Package:           pkg,
			Version:           meta.version,
			PluginDownloadURL: meta.pluginDownloadURL,
		})
	}

	return plugins, nil
}

type ResourceInfo struct {
	Pkg         Package
	TypeName    CanonicalTypeToken
	PackageName string
}

func resolvePkgName(typeString string) string {
	typeParts := strings.Split(typeString, ":")

	// If it's pulumi:providers:aws, the package name is the last label:
	if len(typeParts) == 3 && typeParts[0] == "pulumi" && typeParts[1] == "providers" {
		return typeParts[2]
	}

	return typeParts[0]
}

func ResolveResource(typeString string, packages PackageMap) (*ResourceInfo, error) {
	typeParts := strings.Split(typeString, ":")
	if len(typeParts) < 2 || len(typeParts) > 3 {
		return nil, fmt.Errorf("invalid type token %q", typeString)
	}

	packageName := resolvePkgName(typeString)
	pkg, found := packages[packageName]
	if !found {
		return nil, fmt.Errorf("resource provider %q not found", packageName)
	}
	canonicalName, err := pkg.ResolveResource(typeString)
	if err != nil {
		return nil, err
	}

	return &ResourceInfo{
		Pkg:         pkg,
		TypeName:    canonicalName,
		PackageName: packageName,
	}, nil
}

func ResolveFunction(typeString string, packages PackageMap) (CanonicalTypeToken, error) {
	typeParts := strings.Split(typeString, ":")
	if len(typeParts) < 2 || len(typeParts) > 3 {
		return "", fmt.Errorf("invalid type token %q", typeString)
	}

	packageName := resolvePkgName(typeString)
	pkg, found := packages[packageName]
	if !found {
		return "", fmt.Errorf("resource provider %q not found", packageName)
	}
	canonicalName, err := pkg.ResolveFunction(typeString)
	if err != nil {
		return "", err
	}

	return canonicalName, nil
}

type resourcePackage struct {
	*schema.Package
}

func (p resourcePackage) ResolveResource(typeName string) (CanonicalTypeToken, error) {
	typeParts := strings.Split(typeName, ":")
	if len(typeParts) < 2 || len(typeParts) > 3 {
		return "", fmt.Errorf("invalid type token %q", typeName)
	}

	// pulumi:providers:$pkgName
	if len(typeParts) == 3 &&
		typeParts[0] == "pulumi" &&
		typeParts[1] == "providers" &&
		typeParts[2] == p.Package.Name {
		return CanonicalTypeToken(p.Provider.Token), nil
	}

	if res, found := p.GetResource(typeName); found {
		return CanonicalTypeToken(res.Token), nil
	}

	// If the provided type token is `$pkg:type`, expand it to `$pkg:index:type` automatically. We
	// may well want to handle this more fundamentally in Pulumi itself to avoid the need for
	// `:index:` ceremony quite generally.
	if len(typeParts) == 2 {
		alternateName := fmt.Sprintf("%s:index:%s", typeParts[0], typeParts[1])
		if res, found := p.GetResource(alternateName); found {
			return CanonicalTypeToken(res.Token), nil
		}
		typeParts = []string{typeParts[0], "index", typeParts[1]}
	}

	// A legacy of classic providers is resources with names like `aws:s3/bucket:Bucket`. Here, we
	// allow the user to enter `aws:s3:Bucket`, and we interpolate in the 3rd label, camel cased.
	if len(typeParts) == 3 {
		repeatedSection := strcase.ToLowerCamel(typeParts[2])
		alternateName := fmt.Sprintf("%s:%s/%s:%s", typeParts[0], typeParts[1], repeatedSection, typeParts[2])
		if res, found := p.GetResource(alternateName); found {
			return CanonicalTypeToken(res.Token), nil
		}
	}

	return "", fmt.Errorf("unable to find resource type %q in resource provider %q", typeName, p.Name)
}

func (p resourcePackage) ResolveFunction(typeName string) (CanonicalTypeToken, error) {
	typeParts := strings.Split(typeName, ":")
	if len(typeParts) < 2 || len(typeParts) > 3 {
		return "", fmt.Errorf("invalid type token %q", typeName)
	}

	if res, found := p.GetFunction(typeName); found {
		return CanonicalTypeToken(res.Token), nil
	}

	// If the provided type token is `$pkg:type`, expand it to `$pkg:index:type` automatically. We
	// may well want to handle this more fundamentally in Pulumi itself to avoid the need for
	// `:index:` ceremony quite generally.
	if len(typeParts) == 2 {
		alternateName := fmt.Sprintf("%s:index:%s", typeParts[0], typeParts[1])
		if res, found := p.GetFunction(alternateName); found {
			return CanonicalTypeToken(res.Token), nil
		}
		typeParts = []string{typeParts[0], "index", typeParts[1]}
	}

	// A legacy of classic providers is resources with names like `aws:s3/bucket:Bucket`. Here, we
	// allow the user to enter `aws:s3:Bucket`, and we interpolate in the 3rd label, camel cased.
	if len(typeParts) == 3 {
		repeatedSection := strcase.ToLowerCamel(typeParts[2])
		alternateName := fmt.Sprintf("%s:%s/%s:%s", typeParts[0], typeParts[1], repeatedSection, typeParts[2])
		if res, found := p.GetFunction(alternateName); found {
			return CanonicalTypeToken(res.Token), nil
		}
	}

	return "", fmt.Errorf("unable to find function %q in resource provider %q", typeName, p.Name)
}

func (p resourcePackage) IsComponent(typeName CanonicalTypeToken) (bool, error) {
	if res, found := p.GetResource(string(typeName)); found {
		return res.IsComponent, nil
	}
	return false, fmt.Errorf("unable to find resource type %q in resource provider %q", typeName, p.Name)
}

func NewResourcePackageMap(plugins []Plugin) (*plugin.Context, PackageMap, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	sink := diag.DefaultSink(os.Stderr, os.Stderr, diag.FormatOptions{})
	pluginCtx, err := plugin.NewContext(sink, sink, nil, nil, cwd, nil, true, nil)
	if err != nil {
		return nil, nil, err
	}

	pulumiLoader := schema.NewPluginLoader(pluginCtx.Host)

	packages := map[string]Package{}
	for _, p := range plugins {
		var version *semver.Version
		if p.Version != "" {
			parsedVersion, err := semver.ParseTolerant(p.Version)
			if err != nil {
				return nil, nil, err
			}
			version = &parsedVersion
		}
		pkg, err := pulumiLoader.LoadPackage(p.Package, version)
		if err != nil {
			return nil, nil, err
		}
		packages[p.Package] = resourcePackage{pkg}
	}

	return pluginCtx, packages, nil
}
