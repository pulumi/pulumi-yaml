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
			Name: "pkg",
		},
		{
			Name:        "pkg2",
			Version:     "1.2",
			DownloadURL: "github://api.github.com/pulumiverse",
		},
		{
			Name: "base",
			Parameterization: &ParameterizationDecl{
				Name:    "pkg",
				Version: "1.0.0",
				Value:   "cGtn",
			},
		},
	}

	actual, err := SearchPackageDecls("testdata")
	require.NoError(t, err)
	require.ElementsMatch(t, expected, actual)
}
