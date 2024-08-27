// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	pcodegen "github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
)

var (
	examplesPath     = makeAbs(filepath.Join("..", "..", "examples"))
	outDir           = makeAbs("transpiled_examples")
	schemaLoadPath   = makeAbs(filepath.Join("..", "pulumiyaml", "testing", "test", "testdata"))
	rootPluginLoader = mockPackageLoader{newPluginLoader()}

	// failingExamples examples are known to not produce valid PCL.
	failingExamples = []string{
		"stackreference-consumer",
		// PCL does not have stringAssets
		"getting-started",
	}

	pclBindOpts = map[string][]pcl.BindOption{
		// pulumi/pulumi#11572
		"azure-app-service": {pcl.SkipResourceTypechecking},

		"aws-static-website": {pcl.SkipResourceTypechecking},
	}
)

func makeAbs(path string) string {
	out, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return out
}

// TestGenerateExamples transpiles and and checks all tests in the examples
// folder.
//
// This test expects the examples folder to have the following structure
// examples/
//
//	${test-name}/
//	  Pulumi.yaml
//
// A folder without Pulumi.yaml will signal an error. Other files are ignored.
//
// The same PULUMI_ACCEPT idiom is used for example tests. After adding a new
// test, run:
//
// ```sh
// PULUMI_ACCEPT=true go test --run=TestGenerateExamples ./...
// ```
//
// This will add the current transpile result to the known results.
func TestGenerateExamples(t *testing.T) {
	t.Parallel()

	examples, err := os.ReadDir(examplesPath)
	require.NoError(t, err)
	//nolint:paralleltest // not directly using the loop variable, but instead using dir.Name() in subtests
	for _, dir := range examples {
		dir := dir

		exampleProjectDir := filepath.Join(examplesPath, dir.Name())

		if _, err := os.Stat(filepath.Join(exampleProjectDir, "Pulumi.yaml")); errors.Is(err, os.ErrNotExist) {
			t.Skip()
		}

		t.Run(dir.Name(), func(t *testing.T) {
			t.Parallel()
			var skip bool
			for _, ex := range failingExamples {
				if ex == dir.Name() {
					skip = true
				}
			}
			if skip {
				t.Skip()
				return
			}
			_, template, diags, err := codegen.LoadTemplate(exampleProjectDir)
			require.NoError(t, err, "Loading project %v", dir)
			require.False(t, diags.HasErrors(), diags.Error())
			// TODO: update examples to use `config` instead of `configuration`. For now, filter out the "`configuration` field is deprecated" warning.
			var filteredDiags hcl.Diagnostics
			for _, d := range diags {
				if strings.Contains(d.Summary, "`configuration` field is deprecated") {
					continue
				}
				filteredDiags = append(filteredDiags, d)
			}
			assert.Len(t, filteredDiags, 0, "Should have neither warnings nor errors")
			if t.Failed() {
				return
			}

			dirName := dir.Name()
			pclFileName := dirName + ".pp"
			pcl, tdiags, err := getValidPCLFile(t, template, pclFileName)
			if pcl != nil {
				// If there wasn't an error, we write out the program file, even if it is invalid PCL.
				writeOrCompare(t, filepath.Join(outDir, dirName+"-pp"), map[string][]byte{pclFileName: pcl})
			}
			require.NoError(t, err)
			require.False(t, tdiags.HasErrors(), tdiags.Error())
		})
	}
}

func newPluginLoader() schema.ReferenceLoader {
	return schema.NewPluginLoader(utils.NewHost(schemaLoadPath))
}

type mockPackageLoader struct{ schema.ReferenceLoader }

func (l mockPackageLoader) LoadPackage(ctx context.Context, descriptor *schema.PackageDescriptor) (pulumiyaml.Package, error) {
	pkg, err := schema.LoadPackageReferenceV2(ctx, l.ReferenceLoader, descriptor)
	if err != nil {
		m := fmt.Sprintf(`Only plugin versions found under %q can currently be loaded`, schemaLoadPath)
		return nil, fmt.Errorf("mockPackageLoader(name=%q, version=%v) failed: %w\n%s", descriptor.Name, descriptor.Version, err, m)
	}
	return pulumiyaml.NewResourcePackage(pkg), nil
}

func (l mockPackageLoader) Close() {}

func getValidPCLFile(t *testing.T, file *ast.TemplateDecl, fileName string) ([]byte, hcl.Diagnostics, error) {
	// nil runner passed in since template is not executed and we can use pkgLoader
	_, tdiags, err := pulumiyaml.PrepareTemplate(file, nil, rootPluginLoader)
	if err != nil {
		return nil, tdiags.HCL(), err
	}

	templateBody, tdiags := codegen.ImportTemplate(file, rootPluginLoader)
	diags := tdiags.HCL()
	if tdiags.HasErrors() {
		return nil, diags, nil
	}
	program := fmt.Sprintf("%v", templateBody)
	parser := hclsyntax.NewParser()
	if err := parser.ParseFile(strings.NewReader(program), fileName); err != nil {
		return nil, diags, err
	}
	diags = diags.Extend(parser.Diagnostics)
	bindOpts := append(pclBindOpts[strings.TrimSuffix(fileName, ".pp")],
		pcl.Loader(rootPluginLoader.ReferenceLoader))
	_, pdiags, err := pcl.BindProgram(parser.Files, bindOpts...)
	if err != nil {
		return []byte(program), diags, err
	}
	diags = diags.Extend(pdiags)
	if diags.HasErrors() {
		return []byte(program), diags, nil
	}
	return []byte(program), diags, nil

}

type ConvertFunc = func(t *testing.T, projectDir string)
type CheckFunc = func(t *testing.T, projectDir string, deps pcodegen.StringSet)

func writeOrCompare(t *testing.T, dir string, files map[string][]byte) {
	pulumiAccept := cmdutil.IsTruthy(os.Getenv("PULUMI_ACCEPT"))
	for path, bytes := range files {
		path = filepath.Join(dir, filepath.FromSlash(path))
		if pulumiAccept {
			err := os.MkdirAll(filepath.Dir(path), 0700)
			require.NoError(t, err)
			err = os.WriteFile(path, bytes, 0600)
			require.NoError(t, err)
		} else {
			expected, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, string(expected), string(bytes), "File mismatch")
		}
	}
}

type LanguageList struct {
	list []string
}

func AllLanguages() LanguageList {
	return Dotnet.And(Golang).And(Python).And(Nodejs)
}

var (
	Dotnet = LanguageList{[]string{"dotnet"}}
	Golang = LanguageList{[]string{"go"}}
	Nodejs = LanguageList{[]string{"nodejs"}}
	Python = LanguageList{[]string{"python"}}
)

func (ll LanguageList) Has(lang string) bool {
	for _, l := range ll.list {
		if l == lang {
			return true
		}
	}
	return false
}

func (ll LanguageList) And(other LanguageList) LanguageList {
	out := ll
	for _, l := range other.list {
		if !ll.Has(l) {
			out.list = append(out.list, l)
		}
	}
	return out
}

func (ll LanguageList) Except(other LanguageList) LanguageList {
	var out LanguageList
	for _, l := range ll.list {
		if !other.Has(l) {
			out.list = append(out.list, l)
		}
	}
	return out
}
