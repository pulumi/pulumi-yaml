package pulumiformation

import "strings"

// PackageVersion is a package name and a possibly empty version.
type PackageVersion struct {
	Package string
	Version string
}

// GetReferencedPackages returns the packages and (if provided) versions for each referenced provider used in the program.
func GetReferencedPackages(tmpl *Template) []PackageVersion {
	// TODO: Should this de-dupe providers?
	var pkgs []PackageVersion
	for _, res := range tmpl.Resources {
		typeParts := strings.Split(res.Type, ":")
		if len(typeParts) == 3 && typeParts[0] == "pulumi" && typeParts[1] == "providers" {
			// If it's pulumi:providers:aws, use the third part.
			pkgs = append(pkgs, PackageVersion{
				Package: typeParts[2],
				Version: res.Version,
			})
		} else {
			// Else if it's aws:s3/bucket:Bucket, use the first part.
			pkgs = append(pkgs, PackageVersion{
				Package: typeParts[0],
				Version: res.Version,
			})
		}
	}
	return pkgs
}
