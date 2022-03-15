// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

// Plugin is a package name and a possibly empty version.
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
		typeParts := strings.Split(res.Type.Value, ":")
		version := res.Options.Version.GetValue()
		pluginDownloadURL := res.Options.PluginDownloadURL.GetValue()
		var pkg string
		if len(typeParts) == 3 && typeParts[0] == "pulumi" && typeParts[1] == "providers" {
			// If it's pulumi:providers:aws, use the third part.
			pkg = typeParts[2]
		} else {
			// Else if it's aws:s3/bucket:Bucket, use the first part.
			pkg = typeParts[0]
		}
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
