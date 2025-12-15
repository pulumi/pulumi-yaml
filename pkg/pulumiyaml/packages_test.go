// Copyright 2025, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input         string
		expected      string
		expectedError string
		found         bool
	}{
		{
			input:    "gcp:index:IAMMember",
			expected: "gcp:index:IAMMember",
			found:    true,
		},
		{
			input:    "gcp:IAMMember",
			expected: "gcp:index:IAMMember",
			found:    true,
		},
		{
			input:    "gcp:iam:CamelCaseHere",
			expected: "gcp:iam/camelCaseHere:CamelCaseHere",
			found:    true,
		},
		{
			input: "gcp:iam:IAMMember",
			// The lower casing of leading acronyms here is unfortunate, but is the expected behaviour.
			// see https://github.com/pulumi/pulumi-terraform-bridge/blob/759c5f0f03591f698ababc8a983ec92f4218fe99/pkg/tfbridge/tokens/tokens.go#L45-L62 //nolint:lll
			expected: "gcp:iam/iAMMember:IAMMember",
			found:    true,
		},
		{
			input: ":",
			found: false,
		},
		{
			input:         "nogood",
			expectedError: "invalid type token",
			found:         false,
		},
		{
			input:         "this:isnt:good:either",
			expectedError: "invalid type token",
			found:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			actual, found, err := resolveToken(tt.input, func(tk string) (string, bool, error) {
				if tk == tt.expected {
					return tk, true, nil
				}
				return "", false, nil
			})

			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.found, found)
				require.Equal(t, tt.expected, actual)
			}
		})
	}
}
