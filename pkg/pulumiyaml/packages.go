// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"strings"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

// Plugin is a package name and a possibly empty version.
type Plugin struct {
	Package           string
	Version           string
	PluginDownloadURL string
}

// GetReferencedPlugins returns the packages and (if provided) versions for each referenced provider
// used in the program.
func GetReferencedPlugins(tmpl *ast.TemplateDecl) []Plugin {
	// TODO: Should this de-dupe providers?
	var pkgs []Plugin
	for _, kvp := range tmpl.Resources.GetEntries() {
		res := kvp.Value
		typeParts := strings.Split(res.Type.Value, ":")
		version := ""
		pluginDownloadURL := ""
		if res.Options != nil {
			version = res.Options.Version.GetValue()
			pluginDownloadURL = res.Options.PluginDownloadURL.GetValue()
		}
		if len(typeParts) == 3 && typeParts[0] == "pulumi" && typeParts[1] == "providers" {
			// If it's pulumi:providers:aws, use the third part.
			pkgs = append(pkgs, Plugin{
				Package:           typeParts[2],
				Version:           version,
				PluginDownloadURL: pluginDownloadURL,
			})
		} else {
			// Else if it's aws:s3/bucket:Bucket, use the first part.
			pkgs = append(pkgs, Plugin{
				Package:           typeParts[0],
				Version:           version,
				PluginDownloadURL: pluginDownloadURL,
			})
		}
	}
	return pkgs
}
