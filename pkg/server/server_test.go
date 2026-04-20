// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	codegenrpc "github.com/pulumi/pulumi/sdk/v3/proto/go/codegen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// countingLoaderServer wraps a codegenrpc.LoaderServer and counts GetSchema
// calls per package.
type countingLoaderServer struct {
	codegenrpc.UnsafeLoaderServer
	inner codegenrpc.LoaderServer

	mu    sync.Mutex
	calls map[string]int
}

func (s *countingLoaderServer) GetSchema(
	ctx context.Context, req *codegenrpc.GetSchemaRequest,
) (*codegenrpc.GetSchemaResponse, error) {
	s.mu.Lock()
	s.calls[req.Package]++
	s.mu.Unlock()
	return s.inner.GetSchema(ctx, req)
}

func (s *countingLoaderServer) callCount(pkg string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[pkg]
}

// staticLoader serves pre-imported packages by name.
type staticLoader struct {
	packages map[string]*schema.Package
}

func (l *staticLoader) LoadPackage(pkg string, version *semver.Version) (*schema.Package, error) {
	p, ok := l.packages[pkg]
	if !ok {
		return nil, fmt.Errorf("package %q not found", pkg)
	}
	return p, nil
}

func (l *staticLoader) LoadPackageV2(_ context.Context, desc *schema.PackageDescriptor) (*schema.Package, error) {
	return l.LoadPackage(desc.Name, desc.Version)
}

func (l *staticLoader) LoadPackageReference(pkg string, version *semver.Version) (schema.PackageReference, error) {
	p, err := l.LoadPackage(pkg, version)
	if err != nil {
		return nil, err
	}
	return p.Reference(), nil
}

func (l *staticLoader) LoadPackageReferenceV2(
	_ context.Context, desc *schema.PackageDescriptor,
) (schema.PackageReference, error) {
	return l.LoadPackageReference(desc.Name, desc.Version)
}

// TestGeneratePackageCachesSchemaLoadsRegression verifies that GeneratePackage does not
// make redundant GetSchema calls for the same package. This is a regression
// test for a bug where GeneratePackage passed an uncached loader to
// schema.BindSpec, causing N GetSchema RPCs for a package referenced N times
func TestGeneratePackageCachesSchemaLoadsRegression(t *testing.T) {
	t.Parallel()

	// Build a "dependency" package with several types.
	depVersion := "1.0.0"
	depSpec := schema.PackageSpec{
		Name:    "dep",
		Version: depVersion,
		Types: map[string]schema.ComplexTypeSpec{
			"dep:index:TypeA": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
			"dep:index:TypeB": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "integer"}},
					},
				},
			},
		},
	}

	depPkg, err := schema.ImportSpec(depSpec, nil, schema.ValidationOptions{})
	require.NoError(t, err)

	// Build a "main" package whose resources reference the dependency types many
	// times, simulating how awsx references ~158 AWS types.
	const numResources = 20
	resources := make(map[string]schema.ResourceSpec, numResources)
	for i := range numResources {
		typeName := "TypeA"
		if i%2 == 1 {
			typeName = "TypeB"
		}
		resources[fmt.Sprintf("main:index:Resource%d", i)] = schema.ResourceSpec{
			ObjectTypeSpec: schema.ObjectTypeSpec{
				Type: "object",
				Properties: map[string]schema.PropertySpec{
					"prop": {
						TypeSpec: schema.TypeSpec{
							Ref: fmt.Sprintf("/dep/v%s/schema.json#/types/dep:index:%s", depVersion, typeName),
						},
					},
				},
			},
		}
	}

	mainSpec := schema.PackageSpec{
		Name:      "main",
		Version:   "1.0.0",
		Resources: resources,
	}

	mainJSON, err := json.Marshal(mainSpec)
	require.NoError(t, err)

	// Start a gRPC loader server backed by a counting wrapper so we can observe
	// how many GetSchema RPCs GeneratePackage triggers.
	inner := &staticLoader{packages: map[string]*schema.Package{"dep": depPkg}}
	counter := &countingLoaderServer{
		inner: schema.NewLoaderServer(inner),
		calls: make(map[string]int),
	}

	srv := grpc.NewServer()
	codegenrpc.RegisterLoaderServer(srv, counter)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(srv.Stop)

	// Verify the server is reachable.
	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	conn.Close()

	// Call GeneratePackage, which internally creates a LoaderClient and calls
	// schema.BindSpec. Before the fix, BindSpec received an uncached loader and
	// would issue a GetSchema RPC for every cross-package type reference.
	host := &yamlLanguageHost{}
	resp, err := host.GeneratePackage(t.Context(), &pulumirpc.GeneratePackageRequest{
		Directory:    t.TempDir(),
		Schema:       string(mainJSON),
		LoaderTarget: lis.Addr().String(),
	})
	require.NoError(t, err)
	for _, d := range resp.Diagnostics {
		require.NotEqual(t, codegenrpc.DiagnosticSeverity_DIAG_ERROR, d.Severity, d.Summary)
	}

	// With a cached loader, "dep" should be fetched exactly once regardless of
	// how many resources reference it. Without caching this would be numResources.
	assert.Equal(t, 1, counter.callCount("dep"),
		"expected 1 GetSchema call for dep, got %d (caching not working)", counter.callCount("dep"))
}

func TestGetRequiredPackages(t *testing.T) {
	t.Parallel()

	newRequest := func(dir string) *pulumirpc.GetRequiredPackagesRequest {
		return &pulumirpc.GetRequiredPackagesRequest{
			Info: &pulumirpc.ProgramInfo{
				ProgramDirectory: dir,
				EntryPoint:       ".",
				Options:          &structpb.Struct{},
			},
		}
	}

	t.Run("Pulumi.yaml", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Pulumi.yaml"), []byte(`
name: test-project
runtime: yaml
resources:
  bucket:
    type: aws:s3:Bucket
  pet:
    type: random:RandomPet
    properties:
      length: 2
`), 0o600))

		host := &yamlLanguageHost{templateCache: make(map[string]templateCacheEntry)}
		resp, err := host.GetRequiredPackages(t.Context(), newRequest(dir))
		require.NoError(t, err)

		assert.Equal(t, []*pulumirpc.PackageDependency{
			{Kind: "resource", Name: "aws"},
			{Kind: "resource", Name: "random"},
		}, resp.Packages)
	})

	t.Run("PulumiPlugin.yaml", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "PulumiPlugin.yaml"), []byte(`
name: my-component
runtime: yaml
components:
  myComponent:
    inputs:
      length:
        type: integer
    resources:
      pet:
        type: random:RandomPet
        properties:
          length: ${length}
    outputs:
      name: ${pet.id}
`), 0o600))

		host := &yamlLanguageHost{templateCache: make(map[string]templateCacheEntry)}
		resp, err := host.GetRequiredPackages(t.Context(), newRequest(dir))
		require.NoError(t, err)

		assert.Equal(t, []*pulumirpc.PackageDependency{
			{Kind: "resource", Name: "random"},
		}, resp.Packages)
	})
}
