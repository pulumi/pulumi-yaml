// Copyright 2025, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"context"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
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

// capturingLoader records the descriptor passed to LoadPackage so tests can
// assert on what loadPackage resolved.
type capturingLoader struct {
	got *schema.PackageDescriptor
}

func (l *capturingLoader) LoadPackage(_ context.Context, descriptor *schema.PackageDescriptor) (Package, error) {
	l.got = descriptor
	return MockPackage{}, nil
}

func (l *capturingLoader) Close() {}

// TestLoadPackageOverrides verifies that per-resource version and
// pluginDownloadURL values override a shared descriptor without mutating it.
func TestLoadPackageOverrides(t *testing.T) {
	t.Parallel()

	sharedVersion := semver.Version{Major: 1}
	const sharedURL = "https://example.com/shared"

	overrideVersion := semver.Version{Major: 2, Minor: 5}
	const overrideURL = "https://example.com/override"

	tests := []struct {
		name              string
		version           *semver.Version
		pluginDownloadURL string
		expected          schema.PackageDescriptor
	}{
		{
			name:     "version override wins",
			version:  &overrideVersion,
			expected: schema.PackageDescriptor{Name: "test", Version: &overrideVersion, DownloadURL: sharedURL},
		},
		{
			name:              "pluginDownloadURL override wins",
			pluginDownloadURL: overrideURL,
			expected:          schema.PackageDescriptor{Name: "test", Version: &sharedVersion, DownloadURL: overrideURL},
		},
		{
			name:              "both overrides win",
			version:           &overrideVersion,
			pluginDownloadURL: overrideURL,
			expected:          schema.PackageDescriptor{Name: "test", Version: &overrideVersion, DownloadURL: overrideURL},
		},
		{
			name:     "no overrides uses shared descriptor",
			expected: schema.PackageDescriptor{Name: "test", Version: &sharedVersion, DownloadURL: sharedURL},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			shared := &schema.PackageDescriptor{
				Name:        "test",
				Version:     &sharedVersion,
				DownloadURL: sharedURL,
			}
			descriptors := map[tokens.Package]*schema.PackageDescriptor{
				"test": shared,
			}

			loader := &capturingLoader{}
			_, err := loadPackage(t.Context(), loader, descriptors,
				"test:resource:type", tt.version, tt.pluginDownloadURL,
			)
			require.NoError(t, err)
			require.NotNil(t, loader.got)
			require.Equal(t, tt.expected, *loader.got)

			// The shared descriptor must not be mutated by loadPackage.
			require.Equal(t, schema.PackageDescriptor{
				Name:        "test",
				Version:     &sharedVersion,
				DownloadURL: sharedURL,
			}, *shared)
		})
	}
}
