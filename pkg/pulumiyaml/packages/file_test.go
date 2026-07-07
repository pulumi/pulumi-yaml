// Copyright 2024, Pulumi Corporation.  All rights reserved.

package packages

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchPackageLocks(t *testing.T) {
	t.Parallel()

	expected := []PackageDecl{
		{
			PackageDeclarationVersion: 1,
			Name:                      "pkg",
		},
		{
			PackageDeclarationVersion: 1,
			Name:                      "pkg2",
			Version:                   "1.2",
			DownloadURL:               "github://api.github.com/pulumiverse",
		},
		{
			PackageDeclarationVersion: 1,
			Name:                      "base",
			Parameterization: &ParameterizationDecl{
				Name:    "pkg",
				Version: "1.0.0",
				Value:   "cGtn",
			},
		},
	}

	actual, err := SearchPackageDecls("testdata/good")
	require.NoError(t, err)
	require.ElementsMatch(t, expected, actual)
}

func TestValidateExtensionRequiresVersion(t *testing.T) {
	t.Parallel()

	decl := PackageDecl{
		PackageDeclarationVersion: 1,
		Name:                      "base",
		Extension:                 &ParameterizationDecl{Name: "ext"},
	}
	ok, err := decl.Validate()
	assert.True(t, ok)
	require.ErrorContains(t, err, "extension version is required")
}

func TestValidateRejectsBothParameterizationFlavors(t *testing.T) {
	t.Parallel()

	decl := PackageDecl{
		PackageDeclarationVersion: 1,
		Name:                      "base",
		Parameterization:          &ParameterizationDecl{Name: "pkg", Version: "1.0.0"},
		Extension:                 &ParameterizationDecl{Name: "ext", Version: "2.0.0"},
	}
	ok, err := decl.Validate()
	assert.True(t, ok)
	require.ErrorContains(t, err, "package cannot declare both parameterization and extension")
}

func TestToPackageDescriptorsExtension(t *testing.T) {
	t.Parallel()

	decls := []PackageDecl{{
		PackageDeclarationVersion: 1,
		Name:                      "base",
		Version:                   "0.0.1",
		Extension: &ParameterizationDecl{
			Name:    "ext",
			Version: "2.0.0",
			Value:   "ZXh0",
		},
	}}

	descriptors, err := ToPackageDescriptors(decls)
	require.NoError(t, err)

	// An extension keeps the base provider's name as its namespace key.
	desc, ok := descriptors[tokens.Package("base")]
	require.True(t, ok, "extension descriptor should be keyed by the base provider name")
	assert.Equal(t, "base", desc.Name)
	require.NotNil(t, desc.Parameterization)
	assert.Equal(t, "ext", desc.Parameterization.Name)
	assert.Equal(t, "2.0.0", desc.Parameterization.Version.String())
	assert.Equal(t, []byte("ext"), desc.Parameterization.Value)
}

func TestSearchPackageLocks_Bad(t *testing.T) {
	t.Parallel()

	_, err := SearchPackageDecls("testdata/bad")
	require.ErrorContains(t, err, "validating testdata/bad/bad.yaml: package name is required")
}

func TestSearchPackageLocks_BadParam(t *testing.T) {
	t.Parallel()

	_, err := SearchPackageDecls("testdata/bad_param")
	require.ErrorContains(t, err, "validating testdata/bad_param/bad_param.yaml: parameterization version is required")
}
