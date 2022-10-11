// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"path/filepath"
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
