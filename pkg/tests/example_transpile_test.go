// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/resource/deploy/deploytest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	pcodegen "github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/dotnet"
	gogen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	"github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	"github.com/pulumi/pulumi/pkg/v3/codegen/python"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
)

var (
	examplesPath = filepath.Join("..", "..", "examples")
	outDir       = "transpiled_examples"

	// failingExamples examples are known to not produce valid PCL.
	failingExamples = []string{
		"azure-app-service",
		"webserver-json",
		"stackreference-consumer",
	}

	// failingCompile examples are known to produce valid PCL, but produce
	// invalid transpiled code.
	failingCompile = map[string]LanguageList{
		"stackreference-producer": Dotnet.And(Golang),
		"stackreference-consumer": AllLanguages().Except(Python),
		"random":                  Dotnet.And(Nodejs),
		"getting-started":         AllLanguages(),
		"azure-static-website":    AllLanguages(),
		"aws-static-website":      AllLanguages(),
		"webserver":               AllLanguages().Except(Nodejs),
		"azure-container-apps":    AllLanguages(),
		"aws-eks":                 AllLanguages().Except(Python),
	}

	langTests = []ConvertFunc{
		convertTo("nodejs", nodejs.GenerateProgram, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			nodejs.Check(t, filepath.Join(dir, "index.ts"), deps, false)
		}),
		convertTo("python", python.GenerateProgram, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			python.Check(t, filepath.Join(dir, "__main__.py"), deps)
		}),
		convertTo("go", gogen.GenerateProgram, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			gogen.Check(t, filepath.Join(dir, "main.go"), deps, "")
		}),
		convertTo("dotnet", dotnet.GenerateProgram, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			dotnet.Check(t, filepath.Join(dir, "Program.cs"), deps, "")
		}),
	}
)

// TestGenerateExamples transpiles and and checks all tests in the examples
// folder.
//
// This test expects the examples folder to have the following structure
// examples/
//   ${test-name}/
//     Pulumi.yaml
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
	examples, err := ioutil.ReadDir(examplesPath)
	require.NoError(t, err)
	for _, dir := range examples {
		dir := dir
		t.Run(dir.Name(), func(t *testing.T) {
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
			main, err := getMain(filepath.Join(examplesPath, dir.Name()))
			require.NoError(t, err, "Could not get file path")
			template, diags, err := pulumiyaml.LoadFile(main)
			if err == os.ErrNotExist {
				template, diags, err = pulumiyaml.LoadFile(main)
			}
			require.NoError(t, err, "Loading file: %s", main)
			require.False(t, diags.HasErrors(), diags.Error())
			assert.Len(t, diags, 0, "Should have neither warnings nor errors")
			if t.Failed() {
				return
			}

			pcl, tdiags, err := getValidPCLFile(template)
			if pcl != nil {
				// If there wasn't an error, we write out the program file, even if it is invalid PCL.
				writeOrCompare(t, filepath.Join(outDir, dir.Name()), map[string][]byte{"program.pp": pcl})
			}
			require.NoError(t, err)
			require.False(t, tdiags.HasErrors(), tdiags.Error())
			for _, f := range langTests {
				f(t, template, dir.Name())
			}
		})
	}
}

func getMain(dir string) (string, error) {
	attempts := []string{"Main.yaml", "Main.json", "Pulumi.yaml"}
	for _, base := range attempts {
		path := filepath.Join(dir, base)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("getMain: %w", err)
		}
	}
	return "", fmt.Errorf("could not find a main file in '%s'", dir)
}

func pluginHost() plugin.Host {
	schemaLoadPath := filepath.Join("..", "pulumiyaml", "testing", "test", "testdata")
	host := func(pkg tokens.Package, version semver.Version) *deploytest.PluginLoader {
		return deploytest.NewProviderLoader(pkg, version, func() (plugin.Provider, error) {
			return utils.NewProviderLoader(pkg.String())(schemaLoadPath)
		})
	}
	return deploytest.NewPluginHost(nil, nil, nil,
		host("aws", semver.MustParse("4.26.0")),
		host("azure-native", semver.MustParse("1.56.0")),
		host("azure", semver.MustParse("4.18.0")),
		host("kubernetes", semver.MustParse("3.7.2")),
		host("random", semver.MustParse("4.2.0")),
		host("eks", semver.MustParse("0.37.1")),
		host("aws-native", semver.MustParse("0.13.0")),
		host("docker", semver.MustParse("3.1.0")),

		// Extra packages are to satisfy the versioning requirement of aws-eks.
		// While the schemas are not the correct version, we rely on not
		// depending on the difference between them.
		host("aws", semver.MustParse("4.15.0")),
		host("kubernetes", semver.MustParse("3.0.0")),
	)
}

func getValidPCLFile(file *ast.TemplateDecl) ([]byte, hcl.Diagnostics, error) {
	templateBody, tdiags := codegen.ImportTemplate(file)
	diags := hcl.Diagnostics(tdiags)
	if tdiags.HasErrors() {
		return nil, diags, nil
	}
	program := fmt.Sprintf("%v", templateBody)
	parser := hclsyntax.NewParser()
	if err := parser.ParseFile(strings.NewReader(program), "program.pp"); err != nil {
		return nil, diags, err
	}
	diags = diags.Extend(parser.Diagnostics)
	_, pdiags, err := pcl.BindProgram(parser.Files, pcl.PluginHost(pluginHost()))
	if err != nil {
		return []byte(program), diags, err
	}
	diags = diags.Extend(pdiags)
	if diags.HasErrors() {
		return []byte(program), diags, nil
	}
	return []byte(program), diags, nil

}

type ConvertFunc = func(t *testing.T, template *ast.TemplateDecl, dir string)
type CheckFunc = func(t *testing.T, dir string, deps pcodegen.StringSet)

func shouldBeParallel() bool {
	v := os.Getenv("PULUMI_TEST_PARALLEL")
	return v == "" || cmdutil.IsTruthy(v)
}

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

func convertTo(lang string, generator codegen.GenerateFunc, check CheckFunc) ConvertFunc {
	return func(t *testing.T, template *ast.TemplateDecl, name string) {
		writeTo := filepath.Join(outDir, name, lang)
		t.Run(lang, func(t *testing.T) {
			if failingCompile[name].Has(lang) {
				t.Skipf("%s/%s is known to not produce valid code", name, lang)
				return
			}
			if shouldBeParallel() {
				t.Parallel()
			}
			files, diags, err := codegen.ConvertTemplate(template, generator, pluginHost())
			require.NoError(t, err, "Failed to convert")
			require.False(t, diags.HasErrors(), diags.Error())
			writeOrCompare(t, writeTo, files)
			deps := pcodegen.NewStringSet()
			for _, d := range template.Resources.Entries {
				// This will not handle invokes correctly
				urn := strings.Split(d.Value.Type.Value, ":")
				if urn[0] == "pulumi" && urn[1] == "providers" {
					deps.Add(urn[2])
				} else {
					deps.Add(urn[0])
				}
			}

			check(t, writeTo, deps)
		})
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
