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
	"aws:region": "us-east-1",
}}

var org = os.Getenv("PULUMI_TEST_ORG")

func exampleDir(dir string) string {
	return filepath.Join("../../examples/", dir)
}

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
		PrepareProject{func(stackName string, project *engine.Projinfo) error {
			dir, _, err := project.GetPwdMain()
			if err != nil {
				return err
			}

			// TODO: Replace rewriting the file with config setting instead:
			//
			// See: https://github.com/pulumi/pulumi-yaml/issues/6#issuecomment-1028306579
			err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
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
				template = strings.Replace(template, "PLACEHOLDER_ORG_NAME", org, -1)
				template = strings.Replace(template, "PLACEHOLDER_STACK_NAME", sourceStackName, -1)
				// nolint:gosec // temporary file, no secrets, non-executable
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
	})

	return stackName
}

func TestExampleWebserver(t *testing.T) {
	x := exampleDir("webserver")
	testWrapper(t, x, RequireLiveRun, awsConfig)
}

func TestExampleWebserverInvokeJson(t *testing.T) {
	testWrapper(t, exampleDir("webserver-invoke-json"), RequireLiveRun, awsConfig)
}

func TestExampleWebserverInvoke(t *testing.T) {
	testWrapper(t, exampleDir("webserver-invoke"), RequireLiveRun, awsConfig)
}
