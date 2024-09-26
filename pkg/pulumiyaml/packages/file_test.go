// Copyright 2024, Pulumi Corporation.  All rights reserved.

package packages

import (
	"testing"

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
