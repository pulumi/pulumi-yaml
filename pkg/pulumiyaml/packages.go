// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
)

// Package is our external facing term, e.g.: a provider package in the registry. Packages are
// delivered via plugins, and this interface provides enough surface area to get information about
// resources in a package.
type Package interface {
	IsComponent(typeName string) (bool, error)
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

		tinfo, err := resolveTypeInfo(res.Type.Value)
		if err != nil {
			diags.Extend(ast.ExprError(res.Type, fmt.Sprintf("error resolving type of resource %v: %v", kvp.Key.Value, err), ""))
			continue
		}
		pkg := tinfo.packageName
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

type typeInfo struct {
	typeName    string
	packageName string
}

// TODO: use the package loader to resolve the type and address
// https://github.com/pulumi/pulumi-yaml/issues/41
func resolveTypeInfo(typeString string) (*typeInfo, error) {
	var tInfo typeInfo
	typeParts := strings.Split(typeString, ":")
	if len(typeParts) < 2 || len(typeParts) > 3 {
		return nil, fmt.Errorf("invalid type token %q", typeString)
	}
	// If it's pulumi:providers:aws, use the third part and the type is the whole label:
	if len(typeParts) == 3 && typeParts[0] == "pulumi" && typeParts[1] == "providers" {
		tInfo = typeInfo{
			typeName:    typeString,
			packageName: typeParts[2],
		}
	} else if len(typeParts) == 3 {
		// Else if it's, e.g.: aws:s3/bucket:Bucket, the package is the first label, type string is
		// preserved:
		tInfo = typeInfo{
			typeName:    typeString,
			packageName: typeParts[0],
		}
	} else if len(typeParts) == 2 {
		// If the provided type token is `$pkg:type`, expand it to `$pkg:index:type` automatically. We
		// may well want to handle this more fundamentally in Pulumi itself to avoid the need for
		// `:index:` ceremony quite generally. continue
		tInfo = typeInfo{
			typeName:    fmt.Sprintf("%s:index:%s", typeParts[0], typeParts[1]),
			packageName: typeParts[0],
		}
	} else {
		return nil, fmt.Errorf("invalid type token %q", typeString)
	}

	return &tInfo, nil
}

type resourcePackage struct {
	*schema.Package
}

func (p resourcePackage) IsComponent(typeName string) (bool, error) {
	if res, found := p.GetResource(typeName); found {
		return res.IsComponent, nil
	}
	return false, fmt.Errorf("unable to find resource type %v in resource provider %v", typeName, p.Name)
}

func NewResourcePackageMap(template *ast.TemplateDecl) (PackageMap, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	sink := diag.DefaultSink(os.Stderr, os.Stderr, diag.FormatOptions{})
	pluginCtx, err := plugin.NewContext(sink, sink, nil, nil, cwd, nil, true, nil)
	if err != nil {
		return nil, err
	}

	pulumiLoader := schema.NewPluginLoader(pluginCtx.Host)

	plugins, diags := GetReferencedPlugins(template)
	if diags.HasErrors() {
		return nil, diags
	}

	packages := map[string]Package{}
	for _, p := range plugins {
		var version *semver.Version
		if p.Version != "" {
			parsedVersion, err := semver.ParseTolerant(p.Version)
			if err != nil {
				return nil, err
			}
			version = &parsedVersion
		}
		pkg, err := pulumiLoader.LoadPackage(p.Package, version)
		if err != nil {
			return nil, err
		}
		packages[p.Package] = resourcePackage{pkg}
	}

	return packages, nil
}
