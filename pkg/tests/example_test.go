// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/engine"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
)

var awsConfig = StackConfig{map[string]string{
	"aws:region":        "us-east-1",
	"aws-native:region": "us-east-1",
}}

var azureConfig = StackConfig{map[string]string{
	"azure-native:location": "centralus",
}}

var org = os.Getenv("PULUMI_TEST_ORG")

func exampleDir(dir string) string {
	return filepath.Join("../../examples/", dir)
}

//nolint:paralleltest // uses parallel programtest
func TestRandom(t *testing.T) {
	testWrapper(t, exampleDir("random"))
}

//nolint:paralleltest // uses parallel programtest
func TestExampleAwsStaticWebsite(t *testing.T) {
	testWrapper(t, exampleDir("aws-static-website"), RequireLiveRun, awsConfig)
}

//nolint:paralleltest // uses parallel programtest
func TestExampleAwsx(t *testing.T) {
	testWrapper(t, exampleDir("awsx-fargate"), RequireLiveRun, awsConfig)
}

//nolint:paralleltest // uses parallel programtest
func TestExampleAzureStaticWebsite(t *testing.T) {
	t.Skip()
	testWrapper(t, exampleDir("azure-static-website"), RequireLiveRun, azureConfig)
}

//nolint:paralleltest // uses parallel programtest
func TestExampleAzureAppService(t *testing.T) {
	t.Skip()
	testWrapper(t, exampleDir("azure-app-service"), RequireLiveRun, azureConfig)
}

//nolint:paralleltest // uses parallel programtest
func TestExampleGettingStarted(t *testing.T) {
	testWrapper(t, exampleDir("getting-started"), RequireLiveRun, awsConfig)
}

func TestExampleStackreference(t *testing.T) {
	skipOnDryRun(t)

	t.Parallel()

	// Step 1: Stand up a source project and grab its name:
	sourceStackName := stackReferenceSourceProject(t)

	// Requires pulumi access token to exercise the service API for stack references.
	testWrapper(t, exampleDir("stackreference-consumer"),
		NoParallel,
		RequireLiveRun,
		RequireService,
		PrepareProject{func(stackName string, project *engine.Projinfo) error {
			dir, _, err := project.GetPwdMain()
			if err != nil {
				return err
			}

			// TODO: Replace rewriting the file with config setting instead:
			//
			// See: https://github.com/pulumi/pulumi-yaml/issues/6#issuecomment-1028306579
			err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if path == dir {
					return nil
				}
				if info.IsDir() {
					return filepath.SkipDir
				}
				if info.Name() != "Pulumi.yaml" {
					return nil
				}
				bytes, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				template := string(bytes)
				template = strings.ReplaceAll(template, "PLACEHOLDER_ORG_NAME", org)
				template = strings.ReplaceAll(template, "PLACEHOLDER_STACK_NAME", sourceStackName)
				//nolint:gosec // temporary file, no secrets, non-executable
				err = os.WriteFile(path, []byte(template), 0644)
				if err != nil {
					return err
				}

				return nil
			})

			if err != nil {
				return err
			}

			return nil
		}},
		Validator{func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			imageNameOutput, isString := stack.Outputs["referencedImageName"].(string)
			assert.Truef(t, isString, "Expected stack reference generated output to be a string, full outputs: %+v", stack.Outputs)
			assert.Equalf(t, "pulumi/pulumi:latest", imageNameOutput, "Expected stack reference generated output to match the producer stack's output, full outputs: %+v", stack.Outputs)
		}},
	)
}

func stackReferenceSourceProject(t *testing.T) string {
	sourceDir := exampleDir("stackreference-producer")

	stackName := GetStackName(t, sourceDir)

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		StackName:        stackName,
		Dir:              sourceDir,
		PrepareProject:   prepareYamlProject,
		Quick:            true,
		DestroyOnCleanup: true,
		RequireService:   true,
	})

	return stackName
}

//nolint:paralleltest // uses parallel programtest
func TestExampleWebserver(t *testing.T) {
	testWrapper(t, exampleDir("webserver"), RequireLiveRun, ExpectRefreshChanges, awsConfig)
}

//nolint:paralleltest // uses parallel programtest
func TestExampleWebserverJson(t *testing.T) {
	testWrapper(t, exampleDir("webserver-json"), ExpectRefreshChanges, RequireLiveRun, awsConfig)
}

//nolint:paralleltest // uses parallel programtest
func TestExamplePulumiVariable(t *testing.T) {
	testWrapper(t, exampleDir("pulumi-variable"),
		Validator{func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			cwdOutput, isString := stack.Outputs["cwd"].(string)
			assert.True(t, isString)
			assert.True(t, strings.HasSuffix(cwdOutput, "working-dir"))

			stackOutput, isString := stack.Outputs["stack"].(string)
			assert.True(t, isString)
			assert.Equal(t, string(stack.StackName), stackOutput)

			projectOutput, isString := stack.Outputs["project"].(string)
			assert.True(t, isString)
			assert.Equal(t, "pulumi-reserved-variable", projectOutput)
		}},
	)
}
