// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"strings"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

// PackageVersion is a package name and a possibly empty version.
type PackageVersion struct {
	Package           string
	Version           string
	PluginDownloadURL string
}

// GetReferencedPackages returns the packages and (if provided) versions for each referenced provider used in the program.
func GetReferencedPackages(tmpl *ast.TemplateDecl) []PackageVersion {
	// TODO: Should this de-dupe providers?
	var pkgs []PackageVersion
	for _, kvp := range tmpl.Resources.GetEntries() {
		res := kvp.Value
		typeParts := strings.Split(res.Type.Value, ":")
		if len(typeParts) == 3 && typeParts[0] == "pulumi" && typeParts[1] == "providers" {
			// If it's pulumi:providers:aws, use the third part.
			pkgs = append(pkgs, PackageVersion{
				Package:           typeParts[2],
				Version:           res.Version.GetValue(),
				PluginDownloadURL: res.PluginDownloadURL.GetValue(),
			})
		} else {
			// Else if it's aws:s3/bucket:Bucket, use the first part.
			pkgs = append(pkgs, PackageVersion{
				Package:           typeParts[0],
				Version:           res.Version.GetValue(),
				PluginDownloadURL: res.PluginDownloadURL.GetValue(),
			})
		}
	}
	return pkgs
}
