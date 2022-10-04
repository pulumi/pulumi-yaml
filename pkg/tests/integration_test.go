// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func integrationDir(dir string) string {
	return filepath.Join("./testdata", dir)
}

// extractStackName retrieves the stack name from stderr.
func extractStackName(stderr string) string {
	s := strings.SplitN(stderr, "\n", 2)[0]
	s = strings.TrimPrefix(s, "Previewing update (")
	s = strings.TrimSuffix(s, "):")
	return s
}

//nolint:paralleltest // uses parallel programtest
func TestTypecheckFail(t *testing.T) {
	testWrapper(t, integrationDir("type-fail"), ExpectFailure, StderrValidator{
		f: func(t *testing.T, stderr string) {
			// Since the stack name changes each test, we dynamically extract it for
			// comparison.
			stackName := extractStackName(stderr)
			assert.Equal(t, fmt.Sprintf(`Previewing update (%[1]s):
+ pulumi:pulumi:Stack: (create)
    [urn=urn:pulumi:%[1]s::test-type-fail::pulumi:pulumi:Stack::test-type-fail-%[1]s]
`+"\x1b[31mError\x1b[0m"+`: random:index/randomString:RandomString is not assignable from {length: string, lower: number}

Cannot assign '{length: string, lower: number}' to 'random:index/randomString:RandomString':

  length: Cannot assign type 'string' to type 'integer'

  lower: Cannot assign type 'number' to type 'boolean'

Resources:
    + 1 to create

Previewing update (%[1]s):
+ pulumi:pulumi:Stack: (create)
    [urn=urn:pulumi:%[1]s::test-type-fail::pulumi:pulumi:Stack::test-type-fail-%[1]s]
`+"\x1b[31mError\x1b[0m"+`: random:index/randomString:RandomString is not assignable from {length: string, lower: number}

Cannot assign '{length: string, lower: number}' to 'random:index/randomString:RandomString':

  length: Cannot assign type 'string' to type 'integer'

  lower: Cannot assign type 'number' to type 'boolean'

Resources:
    + 1 to create

`, stackName), stderr)
		},
	})

}
