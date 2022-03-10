package tests

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/dotnet"
	gogen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	"github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	"github.com/pulumi/pulumi/pkg/v3/codegen/python"
)

var (
	examplesPath    = filepath.Join("..", "..", "examples")
	failingExamples = []string{
		"webserver",
		"azure-container-apps",
		"azure-app-service",
		"aws-eks",
	}

	failingCompile = map[string]LanguageList{
		"stackreference-producer": Dotnet.And(Golang),
		"stackreference-consumer": AllLanguages().Except(Python),
		"random":                  Dotnet.And(Nodejs),
		"getting-started":         AllLanguages().Except(Golang),
		"azure-static-website":    AllLanguages(),
		"aws-static-website":      AllLanguages(),
	}
	langTests = []ConvertFunc{
		convertTo("nodejs", nodejs.GenerateProgram, func(t *testing.T, dir string) {
			nodejs.Check(t, filepath.Join(dir, "index.ts"), nil, false)
		}),
		convertTo("python", python.GenerateProgram, func(t *testing.T, dir string) {
			python.Check(t, filepath.Join(dir, "__main__.py"), nil)
		}),
		convertTo("go", gogen.GenerateProgram, func(t *testing.T, dir string) {
			gogen.Check(t, filepath.Join(dir, "main.go"), nil, "")
		}),
		convertTo("dotnet", dotnet.GenerateProgram, func(t *testing.T, dir string) {
			dotnet.Check(t, filepath.Join(dir, "Program.cs"), nil, "")
		}),
	}
)

type ConvertFunc = func(t *testing.T, template *ast.TemplateDecl, dir string)
type CheckFunc = func(t *testing.T, dir string)

func convertTo(lang string, generator codegen.GenerateFunc, check CheckFunc) ConvertFunc {
	return func(t *testing.T, template *ast.TemplateDecl, name string) {
		dir := filepath.Join(examplesPath, name, ".test")
		t.Run(lang, func(t *testing.T) {
			files, diags, err := codegen.ConvertTemplate(template, generator)
			require.NoError(t, err, "Failed to convert")
			assert.False(t, diags.HasErrors(), diags.Error())
			dir := filepath.Join(dir, lang)
			for path, bytes := range files {
				path = filepath.Join(dir, filepath.FromSlash(path))
				err = os.MkdirAll(filepath.Dir(path), 0700)
				require.NoError(t, err)
				err = os.WriteFile(path, bytes, 0600)
				require.NoError(t, err)
			}
			if failingCompile[name].Has(lang) {
				t.Skipf("%s/%s is known to not compile", dir, lang)
				return
			}
			check(t, dir)
		})
	}
}

func TestGenerateExamples(t *testing.T) {
	examples, err := ioutil.ReadDir(examplesPath)
	require.NoError(t, err)
	for _, dir := range examples {
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
			main := filepath.Join(examplesPath, dir.Name(), "Pulumi.yaml")
			template, diags, err := pulumiyaml.LoadFile(main)
			require.NoError(t, err, "Loading file: %s", main)
			assert.False(t, diags.HasErrors(), diags.Error())
			for _, f := range langTests {
				f(t, template, dir.Name())
			}
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
