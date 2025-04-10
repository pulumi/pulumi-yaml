// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	ptesting "github.com/pulumi/pulumi/sdk/v3/go/common/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func integrationDir(dir string) string {
	return filepath.Join("./testdata", dir)
}

func TestAbout(t *testing.T) {
	t.Parallel()

	e := ptesting.NewEnvironment(t)
	defer e.DeleteIfNotFailed()

	e.ImportDirectory(integrationDir("about"))

	stdout, stderr := e.RunCommand("pulumi", "about")
	// There should be no "unknown" plugin versions.
	assert.NotContains(t, stdout, "unknown")
	assert.NotContains(t, stderr, "unknown")
}

//nolint:paralleltest // uses parallel programtest
func TestTypeCheckError(t *testing.T) {
	testWrapper(t, integrationDir("type-fail"), ExpectFailure, StderrValidator{
		f: func(t *testing.T, stderr string) {
			assert.Contains(t, stderr,
				`Cannot assign '{length: string, lower: number}' to 'random:index/randomString:RandomString':

  length: Cannot assign type 'string' to type 'integer'

  lower: Cannot assign type 'number' to type 'boolean'
`)
		},
	})
}

//nolint:paralleltest // uses parallel programtest
func TestInvalidResourceObject(t *testing.T) {
	testWrapper(t, integrationDir("invalid-resource-object"), ExpectFailure, StderrValidator{
		f: func(t *testing.T, stderr string) {
			assert.Contains(t, stderr, "resources.badResource must be an object")
		},
	})
}

//nolint:paralleltest // uses parallel programtest
func TestMismatchedConfigType(t *testing.T) {
	testWrapper(t, integrationDir("mismatched-config-type"), ExpectFailure, StderrValidator{
		f: func(t *testing.T, stderr string) {
			assert.Regexp(t, `config key "foo" cannot have conflicting types boolean, number`, stderr)
		},
	})
}

//nolint:paralleltest // uses parallel programtest
func TestProjectConfigRef(t *testing.T) {
	testWrapper(t, integrationDir("project-config-ref"), ExpectFailure, StderrValidator{
		f: func(t *testing.T, stderr string) {
			assert.Contains(t, stderr,
				`resource, variable, or config value "wrong-namespace:foo" not found`)
			assert.False(t, strings.Contains(stderr,
				`resource, variable, or config value "project-config-ref:foo" not found`))
		},
	})
}

//nolint:paralleltest // uses parallel programtest
func TestProjectConfigWithSecret(t *testing.T) {
	testOptions := integration.ProgramTestOptions{
		Dir:             filepath.Join(getCwd(t), "testdata", "project-config-with-secret"),
		PrepareProject:  prepareYamlProject,
		StackName:       "dev",
		SecretsProvider: "default",
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert.NotEmpty(t, stack.Outputs["my-secret"].(map[string]interface{})["ciphertext"])
		},
	}
	integration.ProgramTest(t, &testOptions)
}

//nolint:paralleltest // uses parallel programtest
func TestProjectConfigWithSecretDecrypted(t *testing.T) {
	testOptions := integration.ProgramTestOptions{
		Dir:                    filepath.Join(getCwd(t), "testdata", "project-config-with-secret"),
		PrepareProject:         prepareYamlProject,
		StackName:              "dev",
		SecretsProvider:        "default",
		DecryptSecretsInOutput: true,
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert.Equal(t, stack.Outputs["my-secret"].(map[string]interface{})["plaintext"], "\"password\"")
		},
	}
	integration.ProgramTest(t, &testOptions)
}

//nolint:paralleltest // uses parallel programtest
func TestEnvVarsPassedToExecCommand(t *testing.T) {
	testOptions := integration.ProgramTestOptions{
		Dir:             filepath.Join(getCwd(t), "testdata", "env-vars"),
		Env:             []string{"TEST_ENV_VAR=foobar"},
		PrepareProject:  prepareYamlProject,
		StackName:       "dev",
		SecretsProvider: "default",
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert.Equal(t, "foobar", stack.Outputs["TEST_ENV_VAR"])
			assert.Equal(t, `dev`, stack.Outputs["PULUMI_STACK"])
			assert.Equal(t, `project-env-vars`, stack.Outputs["PULUMI_PROJECT"])
			assert.Equal(t, `organization`, stack.Outputs["PULUMI_ORGANIZATION"])
			assert.EqualValues(t, map[string]interface{}{"project-env-vars:foo": "hello world"}, stack.Outputs["PULUMI_CONFIG"])
		},
	}
	integration.ProgramTest(t, &testOptions)
}

//nolint:paralleltest // uses parallel programtest
func TestEnvVarsKeepConflictingValues(t *testing.T) {
	testOptions := integration.ProgramTestOptions{
		Dir: filepath.Join(getCwd(t), "testdata", "env-vars"),
		Env: []string{
			"PULUMI_PROJECT=bar",
			"PULUMI_ORGANIZATION=foobar",
			"PULUMI_CONFIG=bazz",
		},
		PrepareProject:  prepareYamlProject,
		StackName:       "dev",
		SecretsProvider: "default",
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert.Equal(t, `bar`, stack.Outputs["PULUMI_PROJECT"])
			assert.Equal(t, `foobar`, stack.Outputs["PULUMI_ORGANIZATION"])
			assert.EqualValues(t, "bazz", stack.Outputs["PULUMI_CONFIG"])
		},
	}
	integration.ProgramTest(t, &testOptions)
}

// Test a local provider plugin.
//
//nolint:paralleltest // ProgramTest calls t.Parallel()
func TestLocalPlugin(t *testing.T) {
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Dir: filepath.Join("testdata", "local"),
		LocalProviders: []integration.LocalDependency{
			{Package: "testprovider", Path: "testprovider"},
		},
	})
}

// Test a paramaterized provider.
//
//nolint:paralleltest // ProgramTest calls t.Parallel()
func TestParameterized(t *testing.T) {
	e := ptesting.NewEnvironment(t)
	// We can't use ImportDirectory here because we need to run this in the right directory such that the relative paths
	// work. This also means we don't delete the directory after the test runs.
	var err error
	e.CWD, err = filepath.Abs("testdata/parameterized")
	require.NoError(t, err)

	err = os.RemoveAll(filepath.Join("testdata", "parameterized", "sdk"))
	require.NoError(t, err)

	_, _ = e.RunCommand("pulumi", "package", "gen-sdk", "../../testprovider", "pkg", "--language", "yaml", "--local")

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Dir: filepath.Join("testdata", "parameterized"),
		LocalProviders: []integration.LocalDependency{
			{Package: "testprovider", Path: "testprovider"},
		},
	})
}

//nolint:paralleltest // uses parallel programtest
func TestResourceOrderingWithDefaultProvider(t *testing.T) {
	integration.ProgramTest(t,
		&integration.ProgramTestOptions{
			Dir:                    filepath.Join("testdata", "resource-ordering"),
			SkipUpdate:             true,
			SkipEmptyPreviewUpdate: true,
		})
}

//nolint:paralleltest // uses parallel programtest
func TestResourceSecret(t *testing.T) {
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Dir: filepath.Join("testdata", "resource-secret"),
	})
}

func TestAuthoredComponent(t *testing.T) {
	t.Parallel()

	e := ptesting.NewEnvironment(t)
	defer e.DeleteIfNotFailed()

	e.ImportDirectory(filepath.Join("testdata", "component"))

	e.RunCommand("pulumi", "login", "--cloud-url", e.LocalURL())

	e.CWD = filepath.Join(e.RootPath, "program")
	e.RunCommand("pulumi", "stack", "init", "organization/component-consumer/test")
	e.RunCommand("pulumi", "package", "add", "../provider")
	e.RunCommand("pulumi", "up", "--non-interactive", "--skip-preview", "--yes")

	stdout, _ := e.RunCommand("pulumi", "stack", "output", "randomPet")
	// We expect 4 words separated by dashes.
	require.Equal(t, 4, len(strings.Split(stdout, "-")))
	require.Equal(t, "test-", stdout[:5])

	stdout, _ = e.RunCommand("pulumi", "stack", "output", "randomString")
	require.Len(t, strings.TrimSuffix(stdout, "\n"), 8, fmt.Sprintf("expected %s to have 8 characters", stdout))
}

func TestRemoteComponent(t *testing.T) {
	t.Parallel()

	e := ptesting.NewEnvironment(t)
	defer e.DeleteIfNotFailed()

	e.ImportDirectory(filepath.Join("testdata", "component-consumption-test"))

	e.RunCommand("pulumi", "login", "--cloud-url", e.LocalURL())
	e.RunCommand("pulumi", "stack", "init", "organization/component-consumption-test/test")
	e.RunCommand(
		"pulumi", "package", "add",
		"github.com/pulumi/component-test-providers/test-provider@b39e20e4e33600e33073ccb2df0ddb46388641dc")

	stdout, _ := e.RunCommand("pulumi", "plugin", "ls")
	assert.Contains(t, stdout, "github.com_pulumi_component-test-providers.git")
	assert.Equal(t, 1, strings.Count(stdout, "component-test-providers"))
	e.RunCommand("pulumi", "up", "--non-interactive", "--skip-preview", "--yes")

	stdout, _ = e.RunCommand("pulumi", "plugin", "ls")
	// Make sure we don't have any extra plugins installed. Regression test for
	// https://github.com/pulumi/pulumi-yaml/pull/734.
	assert.Equal(t, 1, strings.Count(stdout, "component-test-providers"))

	stdout, _ = e.RunCommand("pulumi", "stack", "export")

	unmarshalled := make(map[string]any)
	err := json.Unmarshal([]byte(stdout), &unmarshalled)
	require.NoError(t, err)
	deployment := unmarshalled["deployment"].(map[string]any)
	require.NotNil(t, deployment)

	// Make sure the type of the provider is correct.  Regression test for https://github.com/pulumi/pulumi/issues/18877
	resources := deployment["resources"].([]any)
	found := false
	for _, res := range resources {
		r := res.(map[string]any)
		//nolint:lll
		if r["urn"] == "urn:pulumi:test::component-consumption-test::pulumi:providers:tls-self-signed-cert::default_0_0_0_xb39e20e4e33600e33073ccb2df0ddb46388641dc_git_/github.com/pulumi/component-test-providers/test-provider" {
			found = true
			require.Equal(t, "pulumi:providers:tls-self-signed-cert", r["type"])
		}
	}
	require.True(t, found)
}

func TestRemoteComponentTagged(t *testing.T) {
	t.Parallel()

	e := ptesting.NewEnvironment(t)
	defer e.DeleteIfNotFailed()

	e.ImportDirectory(filepath.Join("testdata", "component-consumption-tagged"))

	e.RunCommand("pulumi", "login", "--cloud-url", e.LocalURL())

	e.RunCommand("pulumi", "stack", "init", "organization/component-consumption-tagged/test")
	e.Env = []string{"PULUMI_DISABLE_AUTOMATIC_PLUGIN_ACQUISITION=false"}
	e.RunCommand("pulumi", "install")
	stdout, _ := e.RunCommand("pulumi", "plugin", "ls")
	// Make sure we have exactly the plugin we expect installed
	assert.Equal(t, 1, strings.Count(stdout, "github.com_pulumi_pulumi-yaml.git"))

	e.RunCommand("pulumi", "up", "--non-interactive", "--skip-preview", "--yes")

	stdout, _ = e.RunCommand("pulumi", "plugin", "ls")
	// Make sure random-plugin-component hasn't been installed under that name.
	assert.Equal(t, 0, strings.Count(stdout, "random-plugin-component"))

	stdout, _ = e.RunCommand("pulumi", "stack", "output", "randomPet")
	// We expect 4 words separated by dashes.
	require.Equal(t, 4, len(strings.Split(stdout, "-")))
	require.Equal(t, "test-", stdout[:5])

	stdout, _ = e.RunCommand("pulumi", "stack", "output", "randomString")
	require.Len(t, strings.TrimSuffix(stdout, "\n"), 8, fmt.Sprintf("expected %s to have 8 characters", stdout))
}

func TestPluginDownloadURLUsed(t *testing.T) {
	t.Parallel()

	e := ptesting.NewEnvironment(t)
	defer e.DeleteIfNotFailed()

	e.ImportDirectory(filepath.Join("testdata", "plugin-download-url"))

	e.RunCommand("pulumi", "login", "--cloud-url", e.LocalURL())

	e.RunCommand("pulumi", "stack", "init", "organization/component-consumption-tagged/test")
	e.Env = []string{"PULUMI_DISABLE_AUTOMATIC_PLUGIN_ACQUISITION=false"}

	e.RunCommand("pulumi", "up", "--non-interactive", "--skip-preview", "--yes")

	stdout, _ := e.RunCommand("pulumi", "plugin", "ls")
	// Make sure random-plugin-component hasn't been installed under that name.
	fmt.Println(stdout)
	assert.Equal(t, 0, strings.Count(stdout, "random-plugin-component"))

	stdout, _ = e.RunCommand("pulumi", "stack", "output", "randomPet")
	// We expect 4 words separated by dashes.
	require.Equal(t, 4, len(strings.Split(stdout, "-")))
	require.Equal(t, "test-", stdout[:5])

	stdout, _ = e.RunCommand("pulumi", "stack", "output", "randomString")
	require.Len(t, strings.TrimSuffix(stdout, "\n"), 8, fmt.Sprintf("expected %s to have 8 characters", stdout))
}
