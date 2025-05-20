// Copyright 2025, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_loadProjectFromDir(t *testing.T) {
	testPath, err := filepath.Abs("./testdata/simple_project")
	require.NoError(t, err, "getting absolute path of test data project")
	proj, main, err := loadProjectFromDir(testPath)
	require.NoError(t, err, "loading project should work")
	require.Equal(t, testPath, main, "the test path is where the project lives")
	require.NotNil(t, proj)
	require.Equal(t, "simple", string(proj.Name), "project name is read correctly")
}

func Test_loadProjectFromDir_fromSubdirectory(t *testing.T) {
	testPath, err := filepath.Abs("./testdata/nested_project/starting_point")
	require.NoError(t, err, "getting absolute path of test data project")
	proj, main, err := loadProjectFromDir(testPath)
	require.NoError(t, err, "loading project should work")
	expectedPath, err := filepath.Abs("./testdata/nested_project")
	require.NoError(t, err, "getting absolute path of test data project")
	require.Equal(t, expectedPath, main, "the test path is where the project lives")
	require.NotNil(t, proj)
	require.Equal(t, "nested", string(proj.Name), "project name is read correctly")
}

func Test_loadProjectFromDir_fromSubdirectoryWithMain(t *testing.T) {
	testPath, err := filepath.Abs("./testdata/nested_project_with_main/starting_point")
	require.NoError(t, err, "getting absolute path of test data project")
	proj, main, err := loadProjectFromDir(testPath)
	require.NoError(t, err, "loading project should work")
	expectedPath, err := filepath.Abs("./testdata/nested_project_with_main/actual_project")
	require.NoError(t, err, "getting absolute path of test data project")
	require.Equal(t, expectedPath, main, "the test path is where the project lives")
	require.NotNil(t, proj)
	require.Equal(t, "nested_with_main", string(proj.Name), "project name is read correctly")
}
