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
func TestConfigObjectType(t *testing.T) {
	testWrapper(t, integrationDir("config-object-type"))
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

// TestComponentChildAliasMigration verifies end-to-end that when a stack already exists
// containing a child resource at the *un-prefixed* URN (the shape produced by code from
// before the #957 fix), running `pulumi up` with the current YAML runtime rebinds the
// existing resource via the auto-generated alias instead of creating a fresh one.
//
// The program also declares a top-level resource named `child` of the same type
// (`random:RandomString`) as the un-prefixed inner URN. This pins the alias scope: an
// alias that was not properly scoped to the component's parent-type chain would either
// match this top-level sibling (rebinding the wrong resource and leaving the inner one
// orphaned) or be rejected as ambiguous. A correct URN-based alias only matches the
// inner child because the parent-type chains differ.
func TestComponentChildAliasMigration(t *testing.T) {
	t.Parallel()

	e := ptesting.NewEnvironment(t)
	defer e.DeleteIfNotFailed()

	e.ImportDirectory(filepath.Join("testdata", "component-alias-migration"))

	e.RunCommand("pulumi", "login", "--cloud-url", e.LocalURL())

	e.CWD = filepath.Join(e.RootPath, "program")
	e.RunCommand("pulumi", "stack", "init", "organization/component-alias-test/test")
	e.RunCommand("pulumi", "package", "add", "../provider")

	// Step 1: initial up. Under the current code this produces:
	//   - a top-level `child` at  ...pulumi:Stack$random:RandomString::child
	//   - an inner child at      ...withChild$random:RandomString::myComp-child
	e.RunCommand("pulumi", "up", "--non-interactive", "--skip-preview", "--yes")

	originalComponentChild, _ := e.RunCommand("pulumi", "stack", "output", "componentChild")
	originalComponentChild = strings.TrimSuffix(originalComponentChild, "\n")
	require.Len(t, originalComponentChild, 8, "component child output should be 8 characters")

	originalTopLevelChild, _ := e.RunCommand("pulumi", "stack", "output", "topLevelChild")
	originalTopLevelChild = strings.TrimSuffix(originalTopLevelChild, "\n")
	require.Len(t, originalTopLevelChild, 8, "top-level child output should be 8 characters")

	// Step 2: rewrite the stack state so the inner child sits at the un-prefixed URN
	// (".../withChild$random:RandomString::child"), simulating a stack created before the
	// #957 fix. We deliberately don't touch the top-level `child`, whose URN already ends
	// in `::child` but is parented by the Stack type rather than the component type.
	stateJSON, _ := e.RunCommand("pulumi", "stack", "export")
	const prefixedSuffix = "$random:index/randomString:RandomString::myComp-child"
	const unprefixedSuffix = "$random:index/randomString:RandomString::child"
	require.Contains(t, stateJSON, prefixedSuffix,
		"expected initial state to contain the prefixed child URN")
	rewritten := strings.ReplaceAll(stateJSON, prefixedSuffix, unprefixedSuffix)
	require.NotEqual(t, stateJSON, rewritten, "expected state rewrite to change the URN")

	rewrittenPath := filepath.Join(e.CWD, "rewritten-state.json")
	require.NoError(t, os.WriteFile(rewrittenPath, []byte(rewritten), 0o600))
	e.RunCommand("pulumi", "stack", "import", "--file", rewrittenPath)

	// Step 3: run `pulumi up` again. With a correctly-scoped alias URN the engine should
	// recognize only the *inner* un-prefixed `child` (parent type withChild) and rebind
	// it under the new prefixed name. The top-level `child` (parent type Stack) must
	// remain untouched.
	e.RunCommand("pulumi", "up", "--non-interactive", "--skip-preview", "--yes")

	// Both outputs must equal their pre-migration values: the alias correctly migrated
	// the inner child, and the alias did NOT spuriously match the top-level sibling.
	migratedComponentChild, _ := e.RunCommand("pulumi", "stack", "output", "componentChild")
	migratedComponentChild = strings.TrimSuffix(migratedComponentChild, "\n")
	assert.Equal(t, originalComponentChild, migratedComponentChild,
		"expected alias to rebind the inner child; got a fresh random value")

	migratedTopLevelChild, _ := e.RunCommand("pulumi", "stack", "output", "topLevelChild")
	migratedTopLevelChild = strings.TrimSuffix(migratedTopLevelChild, "\n")
	assert.Equal(t, originalTopLevelChild, migratedTopLevelChild,
		"top-level child must be untouched; alias scope leaked outside the component")
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

	// TODO https://github.com/pulumi/pulumi-yaml/issues/1036
	// The provider in our testadata pins the version to 1.30.0 of the pulumi-yaml repo to avoid breakage due to the
	// addition of a symlink that the tar extraction code can't handle.
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

//nolint:paralleltest // uses parallel programtest
func TestResourcePropertiesConfig(t *testing.T) {
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		OrderedConfig: []integration.ConfigValue{
			{
				Key:   "props.length",
				Value: "8",
				Path:  true,
			},
		},
		Dir: filepath.Join("testdata", "resource-properties-config"),
	})
}

//nolint:paralleltest // uses parallel programtest
func TestPropertyAccessOnObjects(t *testing.T) {
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		Dir: filepath.Join("testdata", "property-access-on-objects"),
		OrderedConfig: []integration.ConfigValue{
			{
				Key:   "deploymentSettings.githubBranch",
				Value: "main",
				Path:  true,
			},
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			assert.Equal(t, "main", stack.Outputs["branch"])
		},
	})
}
