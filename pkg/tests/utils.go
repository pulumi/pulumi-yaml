// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/engine"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/assert"
)

func getDryRun() bool {
	return !(os.Getenv("PULUMI_LIVE_TEST") == "true")
}

func skipOnDryRun(t *testing.T) {
	if getDryRun() {
		t.Skipf("Skipping test %s which requires non-dryrun config", t.Name())
	}
}

func prepareYamlProject(*engine.Projinfo) error {
	return nil
}

type testOptions struct {
	requireLiveRun     *bool
	programTestOptions integration.ProgramTestOptions
}

type TestOption interface {
	apply(options *testOptions)
}

type requireLiveRun struct{}

var RequireLiveRun = requireLiveRun{}

func (o requireLiveRun) apply(options *testOptions) {
	options.requireLiveRun = boolRef(true)
}

type noParallel struct{}

var NoParallel = noParallel{}

func (o noParallel) apply(options *testOptions) {
	options.programTestOptions = options.programTestOptions.With(integration.ProgramTestOptions{
		NoParallel: true,
	})
}

type expectRefreshChanges struct{}

var ExpectRefreshChanges = expectRefreshChanges{}

func (o expectRefreshChanges) apply(options *testOptions) {
	options.programTestOptions = options.programTestOptions.With(integration.ProgramTestOptions{
		ExpectRefreshChanges:   true,
		SkipEmptyPreviewUpdate: true,
	})
}

type StackConfig struct{ config map[string]string }

func (o StackConfig) apply(options *testOptions) {
	options.programTestOptions = options.programTestOptions.With(integration.ProgramTestOptions{
		Config: o.config,
	})
}

type PrepareProject struct {
	f func(stackName string, project *engine.Projinfo) error
}

func (o PrepareProject) apply(options *testOptions) {
	priorFunc := options.programTestOptions.PrepareProject
	stackName := options.programTestOptions.StackName
	options.programTestOptions.PrepareProject = func(project *engine.Projinfo) error {
		err := priorFunc(project)
		if err != nil {
			return err
		}
		return o.f(stackName, project)
	}
}

type EditDir struct{ editDir integration.EditDir }

func (o EditDir) apply(options *testOptions) {
	options.programTestOptions.EditDirs = append(options.programTestOptions.EditDirs, o.editDir)
}

type Validator struct {
	f func(t *testing.T, stack integration.RuntimeValidationStackInfo)
}

func (o Validator) apply(options *testOptions) {
	priorFunc := options.programTestOptions.ExtraRuntimeValidation
	options.programTestOptions.ExtraRuntimeValidation = func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
		priorFunc(t, stack)
		o.f(t, stack)
	}
}

func boolRef(val bool) *bool { return &val }

func testWrapper(t *testing.T, dir string, opts ...TestOption) {
	dryrun := getDryRun()

	var testOptions testOptions

	testOptions.programTestOptions = integration.ProgramTestOptions{
		Dir:                      filepath.Join(getCwd(t), dir),
		SkipUpdate:               dryrun,
		SkipRefresh:              dryrun,
		AllowEmptyPreviewChanges: dryrun,
		SkipExportImport:         dryrun,
		ExpectRefreshChanges:     !dryrun,

		PrepareProject: prepareYamlProject,
	}

	// Deterministically assign the stack name to provide to PrepareProject
	testOptions.programTestOptions.StackName = string(testOptions.programTestOptions.GetStackName())

	if !dryrun {
		testOptions.programTestOptions = testOptions.programTestOptions.With(integration.ProgramTestOptions{
			ExtraRuntimeValidation: func(t *testing.T, stackInfo integration.RuntimeValidationStackInfo) {
				if !dryrun {
					assert.NotNil(t, stackInfo.Deployment)
				}
			},
		})
	}

	for _, opt := range opts {
		opt.apply(&testOptions)
	}

	if testOptions.requireLiveRun != nil && *testOptions.requireLiveRun {
		skipOnDryRun(t)
	}

	integration.ProgramTest(t, &testOptions.programTestOptions)
}

func getCwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.FailNow()
	}
	return cwd
}
