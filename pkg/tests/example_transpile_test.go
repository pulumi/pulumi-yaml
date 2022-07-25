// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
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
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
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

	// failingCompile examples are known to produce valid PCL, but produce
	// invalid transpiled code.
	failingCompile = map[string]LanguageList{
		"stackreference-producer": Dotnet.And(Golang),
		"stackreference-consumer": AllLanguages().Except(Python),
		"random":                  Dotnet.And(Nodejs),
		"azure-static-website":    AllLanguages(),
		"aws-static-website":      AllLanguages().Except(Python),
		"webserver":               AllLanguages().Except(Nodejs),
		"azure-container-apps":    AllLanguages(),
		"webserver-json":          AllLanguages().Except(Nodejs),
		"aws-eks":                 AllLanguages().Except(Python), // plain inputs
		"cue-eks":                 AllLanguages().Except(Python), // plain inputs
		"azure-app-service":       Dotnet.And(Golang),
		"pulumi-variable":         AllLanguages().Except(Python),
		"kubernetes":              Golang, // returning string instead of *string in ApplyT
		"readme": (Dotnet.And( // https://github.com/pulumi/pulumi/issues/9642
			Golang)), // https://github.com/pulumi/pulumi/issues/9692
	}

	failingTypecheck = map[string]LanguageList{
		"awsx-fargate": (Nodejs.And( // https://github.com/pulumi/pulumi-awsx/issues/853
			Golang)), // https://github.com/pulumi/pulumi/issues/9664
	}

	yarnLock  sync.Mutex
	langTests = []ConvertFunc{
		convertTo("nodejs", nodejs.GenerateProject, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			// concurrent yarn install commands can fail
			yarnLock.Lock()
			defer yarnLock.Unlock()
			nodejs.TypeCheck(t, filepath.Join(dir, "index.ts"), deps, false)
		}),
		convertTo("python", python.GenerateProject, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			python.Check(t, filepath.Join(dir, "__main__.py"), deps)
		}),
		convertTo("go", gogen.GenerateProject, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			gogen.TypeCheck(t, filepath.Join(dir, "main.go"), deps, "")
		}),
		convertTo("dotnet", dotnet.GenerateProject, func(t *testing.T, dir string, deps pcodegen.StringSet) {
			dotnet.TypeCheck(t, filepath.Join(dir, "Program.cs"), deps, "")
		}),
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
	t.Parallel()

	examples, err := ioutil.ReadDir(examplesPath)
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
			assert.Len(t, diags, 0, "Should have neither warnings nor errors")
			if t.Failed() {
				return
			}

			pcl, tdiags, err := getValidPCLFile(t, template)
			if pcl != nil {
				// If there wasn't an error, we write out the program file, even if it is invalid PCL.
				writeOrCompare(t, filepath.Join(outDir, dir.Name()), map[string][]byte{"program.pp": pcl})
			}
			require.NoError(t, err)
			require.False(t, tdiags.HasErrors(), tdiags.Error())
			for _, testLang := range langTests {
				testLang(t, exampleProjectDir)
			}
		})
	}
}

var defaultPlugins = []pulumiyaml.Plugin{
	{Package: "aws", Version: "5.4.0"},
	{Package: "azure-native", Version: "1.56.0"},
	{Package: "azure", Version: "4.18.0"},
	{Package: "kubernetes", Version: "3.7.2"},
	{Package: "random", Version: "4.2.0"},
	{Package: "eks", Version: "0.40.0"},
	{Package: "aws-native", Version: "0.13.0"},
	{Package: "docker", Version: "3.1.0"},
	{Package: "awsx", Version: "1.0.0-beta.5"},

	// Extra packages are to satisfy the versioning requirement of aws-eks.
	// While the schemas are not the correct version, we rely on not
	// depending on the difference between them.
	{Package: "kubernetes", Version: "3.0.0"},
}

func newPluginLoader() schema.ReferenceLoader {
	host := func(pkg tokens.Package, version semver.Version) *deploytest.PluginLoader {
		return deploytest.NewProviderLoader(pkg, version, func() (plugin.Provider, error) {
			return utils.NewProviderLoader(pkg.String())(schemaLoadPath)
		}, deploytest.WithPath(schemaLoadPath))
	}
	var pluginLoaders []*deploytest.PluginLoader
	for _, p := range defaultPlugins {
		pluginLoaders = append(pluginLoaders, host(tokens.Package(p.Package), semver.MustParse(p.Version)))
	}

	return schema.NewPluginLoader(deploytest.NewPluginHost(nil, nil, nil, pluginLoaders...))
}

type mockPackageLoader struct{ schema.ReferenceLoader }

func (l mockPackageLoader) LoadPackage(name string) (pulumiyaml.Package, error) {
	pkg, err := schema.LoadPackageReference(l.ReferenceLoader, name, nil)
	if err != nil {
		return nil, err
	}
	return pulumiyaml.NewResourcePackage(pkg), nil
}

func (l mockPackageLoader) Close() {}

func getValidPCLFile(t *testing.T, file *ast.TemplateDecl) ([]byte, hcl.Diagnostics, error) {
	templateBody, tdiags := codegen.ImportTemplate(file, rootPluginLoader)
	diags := tdiags.HCL()
	if tdiags.HasErrors() {
		return nil, diags, nil
	}
	program := fmt.Sprintf("%v", templateBody)
	parser := hclsyntax.NewParser()
	if err := parser.ParseFile(strings.NewReader(program), "program.pp"); err != nil {
		return nil, diags, err
	}
	diags = diags.Extend(parser.Diagnostics)
	_, pdiags, err := pcl.BindProgram(parser.Files, pcl.Loader(rootPluginLoader.ReferenceLoader))
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

type projectGeneratorFunc func(directory string, project workspace.Project, p *pcl.Program) error

func convertTo(lang string, generator projectGeneratorFunc, check CheckFunc) ConvertFunc {
	return func(t *testing.T, projectDir string) {
		name := filepath.Base(projectDir)
		writeTo := filepath.Join(outDir, name, lang)
		t.Run(lang, func(t *testing.T) {
			if failingCompile[name].Has(lang) {
				t.Skipf("%s/%s is known to not produce valid code", name, lang)
				return
			}
			t.Parallel()
			var (
				diags hcl.Diagnostics
				err   error
			)
			err = os.MkdirAll(writeTo, 0700)
			require.NoError(t, err, "Failed to create target dir")
			_, template, diags, err := codegen.LoadTemplate(projectDir)
			require.NoError(t, err, "Failed to convert")
			require.False(t, diags.HasErrors(), diags.Error())
			proj, pclProgram, err := codegen.Eject(projectDir, rootPluginLoader.ReferenceLoader)
			require.NoError(t, err, "Failed to eject program")

			tmpDir := t.TempDir()
			err = generator(tmpDir, *proj, pclProgram)
			require.NoError(t, err, "Failed to generate project")

			files := map[string][]byte{}

			err = filepath.WalkDir(tmpDir, func(filePath string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if d.IsDir() {
					if name := d.Name(); name == "node_modules" ||
						name == "__pycache__" ||
						name == "bin" {
						return fs.SkipDir
					}
					return nil
				}

				if !(strings.HasSuffix(d.Name(), ".py") ||
					strings.HasSuffix(d.Name(), ".cs") ||
					strings.HasSuffix(d.Name(), ".go") ||
					strings.HasSuffix(d.Name(), ".ts")) {
					return nil
				}

				// Program.cs is the driver for MyStack.cs, which is what we generate.
				if d.Name() == "Program.cs" {
					return nil
				}

				bytes, err := ioutil.ReadFile(filePath)
				if err != nil {
					return err
				}

				segments := []string{}
				rest := filePath[len(tmpDir):]
				for {
					base := filepath.Base(rest)
					segments = append(segments, base)

					if base == rest {
						break
					}
					rest = filepath.Dir(rest)

				}

				name := path.Join(segments...)
				files[name] = bytes
				return nil
			})
			require.NoError(t, err, "Failed to walk generated files")

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

			if failingTypecheck[name].Has(lang) {
				return
			}

			check(t, tmpDir, deps)
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
