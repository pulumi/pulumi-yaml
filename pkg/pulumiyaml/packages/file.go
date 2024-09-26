// Copyright 2024, Pulumi Corporation.  All rights reserved.

// Package packages contains utilities for working with Pulumi package lock files.
package packages

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"gopkg.in/yaml.v3"
)

// ParameterizationDecl defines the structure for the parameterization values of a package.
type ParameterizationDecl struct {
	// Name is the name of the parameterized package.
	Name string `yaml:"name"`
	// Version is the version of the parameterized package.
	Version string `yaml:"version"`
	// Value is the value of the parameter. Value is a base64 encoded byte array, use SetValue and GetValue to manipulate the actual value.
	Value string `yaml:"value"`
}

// GetValue returns the value of the parameter as a byte array. This is just a helper around base64.StdEncoding.
func (p *ParameterizationDecl) GetValue() ([]byte, error) {
	return base64.StdEncoding.DecodeString(p.Value)
}

// SetValue sets the value of the parameter as a byte array. This is just a helper around base64.StdEncoding.
func (p *ParameterizationDecl) SetValue(value []byte) {
	p.Value = base64.StdEncoding.EncodeToString(value)
}

// PackageDecl defines the structure of a package declaration file.
type PackageDecl struct {
	// PackageDeclarationVersion is the version of the package declaration file.
	PackageDeclarationVersion int `yaml:"packageDeclarationVersion"`
	// Name is the name of the plugin.
	Name string `yaml:"name"`
	// Version is the version of the plugin.
	Version string `yaml:"version,omitempty"`
	// PluginDownloadURL is the URL to download the plugin from.
	DownloadURL string `yaml:"downloadUrl,omitempty"`
	// Parameterization is the parameterization of the package.
	Parameterization *ParameterizationDecl `yaml:"parameterization,omitempty"`
}

// Validate checks if a package declaration is valid. The first return value is a boolean indicating if the package declaration is even a
// package declaration. The second return value is an error if the package declaration is invalid.
func (p *PackageDecl) Validate() (bool, error) {
	// All packages should define their version as 1
	if p.PackageDeclarationVersion != 1 {
		return false, nil
	}

	// All packages need a name
	if p.Name == "" {
		return true, fmt.Errorf("package name is required")
	}

	// If parameterization is not nil, it must be valid.
	if p.Parameterization != nil {
		if p.Parameterization.Name == "" {
			return true, fmt.Errorf("parameterization name is required")
		}
		if p.Parameterization.Version == "" {
			return true, fmt.Errorf("parameterization version is required")
		}
	}

	return true, nil
}

// SearchPackageDecls searches the given directory down recursively for package lock .yaml files.
func SearchPackageDecls(directory string) ([]PackageDecl, error) {
	var packages []PackageDecl
	err := filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// If the file is a directory, skip it.
		if d.IsDir() {
			return nil
		}

		// If the file is not a .yaml file, skip it.
		if filepath.Ext(path) != ".yaml" {
			return nil
		}

		// Read the file.
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var packageDecl PackageDecl
		if err := yaml.Unmarshal(data, &packageDecl); err != nil {
			return fmt.Errorf("unmarshalling %s: %w", path, err)
		}

		// If the file is not valid skip it
		ok, err := packageDecl.Validate()
		if !ok {
			return nil
		}
		if err != nil {
			return fmt.Errorf("validating %s: %w", path, err)
		}

		// Else append it to the list of packages found.
		packages = append(packages, packageDecl)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("searching %s for package declarations: %w", directory, err)
	}
	return packages, nil
}

func ToPackageDescriptors(packages []PackageDecl) (map[tokens.Package]*schema.PackageDescriptor, error) {
	packageDescriptors := make(map[tokens.Package]*schema.PackageDescriptor)
	for _, pkg := range packages {

		name := pkg.Name
		// If parametrized use the parametrized name
		var parameterization *schema.ParameterizationDescriptor
		if pkg.Parameterization != nil {
			name = pkg.Parameterization.Name

			value, err := pkg.Parameterization.GetValue()
			if err != nil {
				return nil, fmt.Errorf("decoding parameterization value for package %s: %w", name, err)
			}

			version, err := semver.Parse(pkg.Parameterization.Version)
			if err != nil {
				return nil, fmt.Errorf("parsing version %s for package %s: %w", pkg.Parameterization.Version, name, err)
			}

			parameterization = &schema.ParameterizationDescriptor{
				Name:    pkg.Parameterization.Name,
				Version: version,
				Value:   value,
			}
		}

		var version *semver.Version
		if pkg.Version != "" {
			v, err := semver.Parse(pkg.Version)
			if err != nil {
				return nil, fmt.Errorf("parsing version %s for package %s: %w", pkg.Version, name, err)
			}
			version = &v
		}

		packageDescriptors[tokens.Package(name)] = &schema.PackageDescriptor{
			Name:             pkg.Name,
			Version:          version,
			DownloadURL:      pkg.DownloadURL,
			Parameterization: parameterization,
		}
	}

	return packageDescriptors, nil
}
