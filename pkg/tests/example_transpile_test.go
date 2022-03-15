// Copyright 2022, Pulumi Corporation.  All rights reserved.

package tests

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v43/github"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	pcodegen "github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/dotnet"
	gogen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	"github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	"github.com/pulumi/pulumi/pkg/v3/codegen/python"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
)

var (
	examplesPath = filepath.Join("..", "..", "examples")

	// failingExamples examples are known to not produce valid PCL.
	failingExamples = []string{
		"webserver",
		"azure-container-apps",
		"azure-app-service",
		"aws-eks",
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
			if dir.Name() == "random" {
				SoftFailf(t, "this is a test")
			}
			main := filepath.Join(examplesPath, dir.Name(), "Pulumi.yaml")
			template, diags, err := pulumiyaml.LoadFile(main)
			require.NoError(t, err, "Loading file: %s", main)
			require.False(t, diags.HasErrors(), diags.Error())
			assert.Len(t, diags, 0, "Should have neither warnings nor errors")
			if t.Failed() {
				return
			}

			pcl, tdiags := getPCLFile(template)
			require.False(t, tdiags.HasErrors())
			writeOrCompare(t, filepath.Join(examplesPath, dir.Name(), ".test"), map[string][]byte{"program.pp": pcl})
			for _, f := range langTests {
				f(t, template, dir.Name())
			}
		})
	}
}

func getPCLFile(file *ast.TemplateDecl) ([]byte, hcl.Diagnostics) {
	templateBody, tdiags := codegen.ImportTemplate(file)
	diags := hcl.Diagnostics(tdiags)
	if tdiags.HasErrors() {
		return nil, diags
	}
	return []byte(fmt.Sprintf("%v", templateBody)), diags

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
		dir := filepath.Join(examplesPath, name, ".test")
		t.Run(lang, func(t *testing.T) {
			if shouldBeParallel() {
				t.Parallel()
			}
			files, diags, err := codegen.ConvertTemplate(template, generator)
			require.NoError(t, err, "Failed to convert")
			require.False(t, diags.HasErrors(), diags.Error())
			dir := filepath.Join(dir, lang)
			if failingCompile[name].Has(lang) {
				t.Skipf("%s/%s is known to not produce valid code", dir, lang)
				return
			}
			writeOrCompare(t, dir, files)
			deps := pcodegen.NewStringSet()
			for _, d := range template.Resources.Entries {
				// This will not handle invokes correctly
				deps.Add(d.Value.Type.Value)
			}

			check(t, dir, deps)
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

// Because we depend on upstream code for yaml2pulumi to work, we want to
// accomplish 3 things:
//
// 1. Test the full pipeline: YAML -> $LANG.
// 2. Only merge when tests are green.
// 3. Not stop development when upstream breaks.
//
// To accomplish this, we introduce the concept of soft failures. Soft failures
// only occur in CI, and do not cause a test to fail. Instead, they post a
// comment that indicates what the failure was.
func SoftFailf(t *testing.T, message string, messageArgs ...interface{}) {
	SoftFail(t, fmt.Sprintf(message, messageArgs...))
}

func SoftFail(t *testing.T, message string) {
	if os.Getenv("CI") != "true" {
		// Not in CI => do nothing
		return
	}
	ctx := context.Background()
	client, owner, repo, number := ghClient()
	t.Logf("Soft failure for %s: %s", t.Name(), message)
	body := fmt.Sprintf("###%s failed\n%s", t.Name(), message)
	// Stack trace
	body += "\n<details><summary>Stacktrace</summary>\n<p>\n"
	body += "\n```"
	body += string(debug.Stack())
	body += "```\n\n</p>\n</details>"
	_, response, err := client.PullRequests.CreateComment(ctx, owner, repo, number,
		&github.PullRequestComment{Body: &body})
	if err != nil {
		panic(err)
	}
	if response.StatusCode != 201 {
		panic(fmt.Errorf("Unexpected response: %d %s", response.StatusCode, response.Status))
	}
}

// Returns an authenticated client, owner, repo, issue_number for the current github CI job.
func ghClient() (github.Client, string, string, int) {
	ghToken := os.Getenv("GITHUB_TOKEN")
	client := github.NewClient(oauth2.NewClient(context.Background(),
		oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})))
	repoAndOwner := strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")
	if len(repoAndOwner) != 2 {
		panic("Failed to get owner and repo")
	}
	owner, repo := repoAndOwner[0], repoAndOwner[1]
	rawGHRef := os.Getenv("GITHUB_REF")
	ghRef := strings.Split(rawGHRef, "/")
	if len(ghRef) != 4 {
		panic(fmt.Sprintf("Invalid github REF: '%s'", rawGHRef))
	}
	num, err := strconv.ParseInt(ghRef[2], 10, 64)
	if err != nil {
		panic(fmt.Errorf("Could not parse PR number: %w", err))
	}
	return *client, owner, repo, int(num)
}
