// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func integrationDir(dir string) string {
	return filepath.Join("./testdata", dir)
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
func TestMismatchedConfigType(t *testing.T) {
	testWrapper(t, integrationDir("mismatched-config-type"), ExpectFailure, StderrValidator{
		f: func(t *testing.T, stderr string) {
			assert.Regexp(t, `config key "foo" cannot have conflicting types boolean, (integer|number)`, stderr)
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
