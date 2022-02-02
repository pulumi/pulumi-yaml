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

type testOptions struct {
	requireLiveRun     *bool
	programTestOptions *integration.ProgramTestOptions
}

type TestOption interface {
	apply(options *testOptions)
}

type RequireLiveRun struct{}

func (o RequireLiveRun) apply(options *testOptions) {
	options.requireLiveRun = boolRef(true)
}

type StackConfig struct{ config map[string]string }

func (o StackConfig) apply(options *testOptions) {
	newOpts := options.programTestOptions.With(integration.ProgramTestOptions{
		Config: o.config,
	})
	options.programTestOptions = &newOpts
}

func boolRef(val bool) *bool { return &val }

func testWrapper(t *testing.T, dir string, opts ...TestOption) {
	dryrun := getDryRun()

	var testOptions testOptions

	testOptions.programTestOptions = &integration.ProgramTestOptions{
		Dir:                      filepath.Join(getCwd(t), dir),
		SkipUpdate:               dryrun,
		SkipRefresh:              dryrun,
		AllowEmptyPreviewChanges: dryrun,
		SkipExportImport:         dryrun,
		ExpectRefreshChanges:     !dryrun,

		PrepareProject: func(*engine.Projinfo) error {
			return nil
		},
		Config: map[string]string{
			"aws:region": "us-east-1",
		},
	}

	if !dryrun {
		newPtOpts := testOptions.programTestOptions.With(integration.ProgramTestOptions{
			ExtraRuntimeValidation: func(t *testing.T, stackInfo integration.RuntimeValidationStackInfo) {

				if !dryrun {
					assert.NotNil(t, stackInfo.Deployment)
				}
			},
		})
		testOptions.programTestOptions = &newPtOpts
	}

	for _, opt := range opts {
		opt.apply(&testOptions)
	}

	if testOptions.requireLiveRun != nil && *testOptions.requireLiveRun && dryrun {
		t.Skipf("Skipping test %s which requires non-dryrun config", t.Name())
	}

	integration.ProgramTest(t, testOptions.programTestOptions)
}

func getCwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.FailNow()
	}
	return cwd
}
