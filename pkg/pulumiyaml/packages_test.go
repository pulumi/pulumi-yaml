// Copyright 2025, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
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
			// The schema names the module-qualified segment "iAMMember"; resolution
			// matches it case-insensitively from the lower-cased shorthand.
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
			// The production resolver matches schema tokens case-insensitively and
			// returns the schema's canonical token, so the mock does the same.
			actual, found, err := resolveToken(tt.input, func(tk string) (string, bool, error) {
				if strings.EqualFold(tk, tt.expected) {
					return tt.expected, true, nil
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

// TestResolveTokenLeadingAcronym guards against a casing-sensitivity regression in
// shorthand token resolution. Provider schemas name the module-qualified segment of a
// token by lower-casing only the first rune of a leading acronym, e.g. the real
// guardduty IPSet resource is "aws:guardduty/iPSet:IPSet". The synthesized segment
// ("ipset") differs in case, so resolution must match case-insensitively.
//
// This drives the real ResolveResource path against a bound schema rather than a
// hand-written resolver, so it exercises the same token lookup used at runtime.
func TestResolveTokenLeadingAcronym(t *testing.T) {
	t.Parallel()

	const canonical = "aws:guardduty/iPSet:IPSet"
	pkg, diags, err := schema.BindSpec(schema.PackageSpec{
		Name:    "aws",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			canonical: {},
		},
	}, schema.NewNullLoader(), schema.ValidationOptions{})
	require.NoError(t, err)
	require.False(t, diags.HasErrors(), "%v", diags)

	tk, err := NewResourcePackage(pkg.Reference()).ResolveResource("aws:guardduty:IPSet")
	require.NoError(t, err)
	require.Equal(t, ResourceTypeToken(canonical), tk)
}

// capturingLoader records every descriptor passed to LoadPackage so tests can
// assert on what loadPackages resolved against.
type capturingLoader struct {
	loaded []schema.PackageDescriptor
}

func (l *capturingLoader) LoadPackage(_ context.Context, descriptor *schema.PackageDescriptor) (Package, error) {
	l.loaded = append(l.loaded, *descriptor)
	return MockPackage{}, nil
}

func (l *capturingLoader) Close() {}

func (l *capturingLoader) loadedBase(name string) bool {
	for _, d := range l.loaded {
		if d.Name == name && d.Parameterization == nil {
			return true
		}
	}
	return false
}

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
			descriptors := map[tokens.Package][]*schema.PackageDescriptor{
				"test": {shared},
			}

			loader := &capturingLoader{}
			_, err := loadPackages(t.Context(), loader, descriptors,
				"test:resource:type", tt.version, tt.pluginDownloadURL,
			)
			require.NoError(t, err)
			require.Len(t, loader.loaded, 1)
			require.Equal(t, tt.expected, loader.loaded[0])

			// The shared descriptor must not be mutated by loadPackage.
			require.Equal(t, schema.PackageDescriptor{
				Name:        "test",
				Version:     &sharedVersion,
				DownloadURL: sharedURL,
			}, *shared)
		})
	}
}

func TestLoadPackagesBaseFallback(t *testing.T) {
	t.Parallel()

	extVersion := semver.MustParse("2.0.0")

	t.Run("empty namespace synthesizes a base", func(t *testing.T) {
		t.Parallel()
		loader := &capturingLoader{}
		_, err := loadPackages(t.Context(), loader,
			map[tokens.Package][]*schema.PackageDescriptor{},
			"aws:s3:Bucket", nil, "",
		)
		require.NoError(t, err)
		require.Len(t, loader.loaded, 1)
		assert.True(t, loader.loadedBase("aws"))
	})

	t.Run("extension-only namespace synthesizes the base", func(t *testing.T) {
		t.Parallel()
		loader := &capturingLoader{}
		descriptors := map[tokens.Package][]*schema.PackageDescriptor{
			"extbase": {{
				Name: "extbase",
				Parameterization: &schema.ParameterizationDescriptor{
					Name:    "myext",
					Version: extVersion,
				},
			}},
		}
		_, err := loadPackages(t.Context(), loader, descriptors, "extbase:Base", nil, "")
		require.NoError(t, err)
		assert.True(t, loader.loadedBase("extbase"),
			"an extension-only namespace must add its base provider as a resolution candidate")
	})

	t.Run("namespace with an explicit base does not synthesize another", func(t *testing.T) {
		t.Parallel()
		loader := &capturingLoader{}
		descriptors := map[tokens.Package][]*schema.PackageDescriptor{
			"extbase": {
				{Name: "extbase"},
				{
					Name: "extbase",
					Parameterization: &schema.ParameterizationDescriptor{
						Name:    "myext",
						Version: extVersion,
					},
				},
			},
		}
		_, err := loadPackages(t.Context(), loader, descriptors, "extbase:Base", nil, "")
		require.NoError(t, err)
		require.Len(t, loader.loaded, 2, "must load exactly the two declared descriptors")
	})

	t.Run("replacement parameterization does not synthesize a base", func(t *testing.T) {
		t.Parallel()
		loader := &capturingLoader{}
		descriptors := map[tokens.Package][]*schema.PackageDescriptor{
			"subpackage": {{
				Name: "parameterized",
				Parameterization: &schema.ParameterizationDescriptor{
					Name:    "subpackage",
					Version: extVersion,
				},
			}},
		}
		_, err := loadPackages(t.Context(), loader, descriptors, "subpackage:HelloWorld", nil, "")
		require.NoError(t, err)
		require.Len(t, loader.loaded, 1,
			"a replacement package serves its whole namespace; no base plugin named after the parameterization exists")
		assert.False(t, loader.loadedBase("subpackage"))
	})
}

// resolvingPackage is a MockPackage that recognizes only the given tokens, so a test can
// observe which package in a namespace a token was routed to.
func resolvingPackage(toks ...string) MockPackage {
	known := make(map[string]bool, len(toks))
	for _, tk := range toks {
		known[tk] = true
	}
	return MockPackage{
		resolveResource: func(typeName string) (ResourceTypeToken, error) {
			if known[typeName] {
				return ResourceTypeToken(typeName), nil
			}
			return "", fmt.Errorf("package does not recognize %q", typeName)
		},
	}
}

// funcLoader loads packages via a per-test callback keyed on the descriptor.
type funcLoader func(*schema.PackageDescriptor) (Package, error)

func (l funcLoader) LoadPackage(_ context.Context, d *schema.PackageDescriptor) (Package, error) {
	return l(d)
}

func (l funcLoader) Close() {}

func TestResolveResourceRouting(t *testing.T) {
	t.Parallel()

	extVersion := semver.MustParse("2.0.0")

	t.Run("extension namespace routes base and extension tokens to their own packages", func(t *testing.T) {
		t.Parallel()
		base := resolvingPackage("extbase:Base")
		ext := resolvingPackage("extbase:Greeting")
		loader := funcLoader(func(d *schema.PackageDescriptor) (Package, error) {
			if d.Parameterization == nil {
				return base, nil
			}
			return ext, nil
		})
		// Thin lock: only the extension is declared, so the base must be synthesized.
		descriptors := map[tokens.Package][]*schema.PackageDescriptor{
			"extbase": {{
				Name:             "extbase",
				Parameterization: &schema.ParameterizationDescriptor{Name: "myext", Version: extVersion},
			}},
		}

		_, tk, desc, err := ResolveResource(t.Context(), loader, descriptors, "extbase:Base", nil, "")
		require.NoError(t, err)
		assert.Equal(t, ResourceTypeToken("extbase:Base"), tk)
		assert.Nil(t, desc.Parameterization,
			"a base token must resolve against the synthesized base, not the extension")

		_, tk, desc, err = ResolveResource(t.Context(), loader, descriptors, "extbase:Greeting", nil, "")
		require.NoError(t, err)
		assert.Equal(t, ResourceTypeToken("extbase:Greeting"), tk)
		require.NotNil(t, desc.Parameterization)
		assert.Equal(t, "myext", desc.Parameterization.Name)
	})

	t.Run("replacement namespace routes to the parameterized package with no base synthesized", func(t *testing.T) {
		t.Parallel()
		repl := resolvingPackage("subpackage:HelloWorld")
		loader := funcLoader(func(d *schema.PackageDescriptor) (Package, error) {
			// A synthesized base would be a plain descriptor named after the parameterization,
			// for which no plugin exists. Fail loudly so the regression surfaces here.
			if d.Parameterization == nil {
				return nil, fmt.Errorf("no plugin named %q exists", d.Name)
			}
			return repl, nil
		})
		descriptors := map[tokens.Package][]*schema.PackageDescriptor{
			"subpackage": {{
				Name:             "parameterized",
				Parameterization: &schema.ParameterizationDescriptor{Name: "subpackage", Version: extVersion},
			}},
		}

		_, tk, desc, err := ResolveResource(t.Context(), loader, descriptors, "subpackage:HelloWorld", nil, "")
		require.NoError(t, err)
		assert.Equal(t, ResourceTypeToken("subpackage:HelloWorld"), tk)
		assert.Equal(t, "parameterized", desc.Name)
	})
}

func TestBuildRegisterPackageRequest(t *testing.T) {
	t.Parallel()

	version := semver.MustParse("1.0.0")
	baseVersion := semver.MustParse("0.0.1")

	t.Run("descriptor without parameterization sets neither field", func(t *testing.T) {
		t.Parallel()
		req := buildRegisterPackageRequest(tokens.Package("random"), &schema.PackageDescriptor{
			Name:    "random",
			Version: &version,
		})
		assert.Equal(t, "random", req.Name)
		assert.Equal(t, "1.0.0", req.Version)
		assert.Nil(t, req.Parameterization)
		assert.Nil(t, req.Extension)
	})

	t.Run("descriptor keyed by the parameterization name sets Parameterization", func(t *testing.T) {
		t.Parallel()
		req := buildRegisterPackageRequest(tokens.Package("pkg"), &schema.PackageDescriptor{
			Name:    "testprovider",
			Version: &baseVersion,
			Parameterization: &schema.ParameterizationDescriptor{
				Name:    "pkg",
				Version: version,
				Value:   []byte("pkg"),
			},
		})
		assert.Equal(t, "testprovider", req.Name)
		assert.Equal(t, "0.0.1", req.Version)
		assert.Nil(t, req.Extension)
		assert.Equal(t, &pulumirpc.Parameterization{
			Name:    "pkg",
			Version: "1.0.0",
			Value:   []byte("pkg"),
		}, req.Parameterization)
	})

	t.Run("descriptor keyed by the base provider name sets Extension", func(t *testing.T) {
		t.Parallel()
		req := buildRegisterPackageRequest(tokens.Package("testprovider"), &schema.PackageDescriptor{
			Name:    "testprovider",
			Version: &baseVersion,
			Parameterization: &schema.ParameterizationDescriptor{
				Name:    "ext",
				Version: version,
				Value:   []byte("ext"),
			},
		})
		assert.Equal(t, "testprovider", req.Name)
		assert.Equal(t, "0.0.1", req.Version)
		assert.Nil(t, req.Parameterization)
		assert.Equal(t, &pulumirpc.Parameterization{
			Name:    "ext",
			Version: "1.0.0",
			Value:   []byte("ext"),
		}, req.Extension)
	})
}
