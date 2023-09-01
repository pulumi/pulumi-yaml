// Copyright 2023, Pulumi Corporation.
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

// gocov is a replacement for the 'go test' command to be used for coverage.
//
// It replicates the behavior of 'go test' as-is with one exception:
// it separates build and execution of the test binary.
//
// # Usage
//
//	gocov [options] [patterns] -- [go test flags]
//
// Arguments following '--' (if any) are passed to the test binary as-is.
// This means that flags you intended to pass to 'go test'
// should be prefixed with 'test.'.
//
// For example:
//
//	# Instead of this:
//	go test -run TestFoo ./...
//
//	# Do this:
//	gocov ./... -- -test.run TestFoo
//
// # gotestsum
//
// To use gocov with gotestsum, use the following command:
//
//	// Include other gocov flags as needed.
//	gotestsum --raw-command -- gocov -test2json ./...
//
// # Why
//
// This is a workaround for Go's integration test coverage tracking
// functionality which, as of Go 1.21,
// does not yet support merging unit and integration test coverage in one run.
//
// Specifically, if you build a coverage-instrumented binary
// and then also run tests with coverage tracking:
//
//	go build -o bin/whatever -cover ./cmd/whatever
//	GOCOVERDIR=$someDir go test -cover
//
// The GOCOVERDIR environment variable will NOT be propagated
// to invocations of bin/whatever made from inside the tests
// because 'go test' will override the environment variable.
// https://github.com/golang/go/blob/c19c4c566c63818dfd059b352e52c4710eecf14d/src/cmd/go/internal/test/test.go#L1337-L1341
//
// To work around this, we need to build the test binary with 'go test'
// and then run it separately, setting GOCOVERDIR ourselves.
//
// See https://github.com/golang/go/issues/51430#issuecomment-1344711300
// and https://dustinspecker.com/posts/go-combined-unit-integration-code-coverage/
// for more details on this workaround.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const _usage = `Usage: gocov [options] [patterns] -- [test flags]

Test flags are passed to the test binary as-is.
Do NOT use the -test.coverprofile flag.
Coverage data is written to the coverage/ directory.
Override with the -coverdir flag.

Options:
`

func main() {
	os.Exit(
		(&mainCmd{
			Stdin:   os.Stdin,
			Stdout:  os.Stdout,
			Stderr:  os.Stderr,
			Getwd:   os.Getwd,
			Environ: os.Environ,
		}).Run(os.Args[1:]),
	)
}

// params specifies the command line parameters.
type params struct {
	// Be more verbose.
	Verbose bool

	// Turn the output into a machine-readable JSON stream
	// using https://pkg.go.dev/cmd/test2json.
	//
	// Note that this is not the same as 'go test -json'.
	Test2JSON bool

	// Enables data race detection in tests.
	Race bool

	// Directory where coverage profiles are written.
	CoverDir string

	// Packages to track coverage for.
	CoverPkg string

	// Flags to pass to the test binary.
	TestFlags []string

	// Patterns of packages to run tests for.
	Patterns []string
}

func (p *params) Parse(args []string) error {
	flag := flag.NewFlagSet("gocov", flag.ContinueOnError)
	flag.Usage = func() {
		fmt.Fprint(flag.Output(), _usage+"\n")
		flag.PrintDefaults()
	}

	// Don't use flag defaults.
	// We'll set them ourselves after parsing.
	flag.BoolVar(&p.Verbose, "v", false, "be more verbose")
	flag.BoolVar(&p.Test2JSON, "test2json", false,
		"turn the output into a machine-readable JSON stream")
	flag.BoolVar(&p.Race, "race", false,
		"enable data race detection in tests")
	flag.StringVar(&p.CoverDir, "coverdir", "",
		"directory where coverage profiles are written (default: coverage)")
	flag.StringVar(&p.CoverPkg, "coverpkg", "",
		"comma-separated list of packages to track coverage for "+
			"(defaults to all packages under test)")

	if err := flag.Parse(args); err != nil {
		return err
	}

	// All remaining arguments until '--' are patterns,
	// and everything after that is test flags.
	args = flag.Args()
	p.Patterns = args // if no '--', then all args are patterns
	for i, arg := range args {
		if arg == "--" {
			p.TestFlags = args[i+1:]
			p.Patterns = args[:i]
			break
		}
	}

	if len(p.Patterns) == 0 {
		flag.Usage()
		return fmt.Errorf("pattern(s) required: use '.' for current package")
	}

	return nil
}

type mainCmd struct {
	debugLog *log.Logger

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Getwd   func() (string, error) // == os.Getwd
	Environ func() []string        // == os.Environ
}

func (cmd *mainCmd) debugf(format string, args ...interface{}) {
	if cmd.debugLog != nil {
		cmd.debugLog.Printf(format, args...)
	}
}

func (cmd *mainCmd) Run(args []string) (exitCode int) {
	var p params
	if err := p.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}

		fmt.Fprintln(cmd.Stderr, "gocov:", err)
		return 1
	}

	if err := cmd.run(&p); err != nil {
		fmt.Fprintln(cmd.Stderr, err)
		return 1
	}
	return 0
}

func (cmd *mainCmd) run(p *params) (err error) {
	if p.Verbose {
		cmd.debugLog = log.New(cmd.Stderr, "", 0)

		// Convenience: if -verbose is set, make sure so is -test.v.
		// (No need to dedupe flags; 'go test' handles this just fine.)
		if !p.Test2JSON {
			p.TestFlags = append(p.TestFlags, "-test.v")
		}
	}
	if p.Test2JSON {
		p.TestFlags = append(p.TestFlags, "-test.v=test2json")
	}

	cwd, err := cmd.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	if p.CoverDir == "" {
		p.CoverDir = filepath.Join(cwd, "coverage")
	}
	if !filepath.IsAbs(p.CoverDir) {
		p.CoverDir = filepath.Join(cwd, p.CoverDir)
	}
	if err := os.MkdirAll(p.CoverDir, 0755); err != nil {
		return fmt.Errorf("set up coverage dir: %w", err)
	}

	binDir, err := os.MkdirTemp("", "go-test-bin-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(binDir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("remove temp dir: %w", rmErr))
		}
	}()

	packages, err := cmd.expandPatterns(p.Patterns)
	if err != nil {
		return fmt.Errorf("expand patterns: %w", err)
	}

	// Import paths of all packages to track coverage for.
	var coverPkgs []string
	if p.CoverPkg != "" {
		// If -coverpkg was set, it may be a pattern like './...'.
		// Expand it to a list of import paths.
		pkgs, err := cmd.expandPatterns(strings.Split(p.CoverPkg, ","))
		if err != nil {
			return fmt.Errorf("expand -coverpkg: %w", err)
		}

		coverPkgs = make([]string, len(pkgs))
		for i, pkg := range pkgs {
			coverPkgs[i] = pkg.ImportPath
		}
	} else {
		// If -coverpkg was not set, then default to all packages under test.
		coverPkgs = make([]string, len(packages))
		for i, pkg := range packages {
			coverPkgs[i] = pkg.ImportPath
		}
	}
	sort.Strings(coverPkgs)

	// Build the test binaries with coverage tracking.
	testBinaries := make([]string, len(packages))
	for i, pkg := range packages {
		req := buildPackageRequest{
			Dir:       binDir,
			Package:   pkg,
			CoverPkgs: coverPkgs,
			Race:      p.Race,
		}
		testBinaries[i], err = cmd.buildPackage(req)
		if err != nil {
			return fmt.Errorf("build package %v: %w", pkg.ImportPath, err)
		}
	}

	// Run the test binaries.
	errs := make([]error, len(packages))
	for i, pkg := range packages {
		testBin := testBinaries[i]
		if testBin == "" {
			// No tests for this package.
			continue
		}

		r := testPackageRequest{
			CoverDir:   p.CoverDir,
			Flags:      p.TestFlags,
			Package:    pkg,
			TestBinary: testBin,
			Test2JSON:  p.Test2JSON,
		}
		if err := cmd.testPackage(r); err != nil {
			errs[i] = fmt.Errorf("test package %v: %w", pkg.ImportPath, err)
		}
	}
	return errors.Join(errs...)
}

type buildPackageRequest struct {
	Dir       string    // destination directory for test binary
	Package   goPackage // package to build tests for
	CoverPkgs []string  // import paths of packages to track coverage for
	Race      bool      // whether to enable data race detection
}

// buildPackage builds the given package and returns the path to its test binary.
// Returns an empty path if the package has no tests.
func (cmd *mainCmd) buildPackage(r buildPackageRequest) (string, error) {
	// Test binaries are typically named $pkg.test after the package name.
	// Since we're placing them all in the same directory,
	// we'll want to ensure there are no name collisions.
	// We can use os.CreateTemp to generate unique names.
	var binPath string
	{
		f, err := os.CreateTemp(r.Dir, r.Package.Name+".test")
		if err != nil {
			return "", fmt.Errorf("create temp file: %w", err)
		}

		binPath = f.Name()
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("close temp file: %w", err)
		}

		if err := os.Remove(binPath); err != nil {
			return "", fmt.Errorf("remove temp file: %w", err)
		}
	}

	args := []string{"test", "-c",
		"-cover",
		"-coverpkg=" + strings.Join(r.CoverPkgs, ","),
		"-o", binPath,
	}
	if r.Race {
		args = append(args, "-race")
	}
	args = append(args, r.Package.ImportPath)

	// Build the test binary with coverage instrumentation.
	buildCmd := exec.Command("go", args...) //nolint:gosec
	buildCmd.Stdout = cmd.Stdout
	buildCmd.Stderr = cmd.Stderr
	buildCmd.Dir = r.Package.Dir
	cmd.debugf("*** %v", buildCmd)
	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("build test binary: %w", err)
	}

	// The binary will not exist if the package has no tests.
	if _, err := os.Stat(binPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("stat test binary: %w", err)
	}

	return binPath, nil
}

type testPackageRequest struct {
	CoverDir   string    // directory to write coverage data to
	Flags      []string  // test flags
	Package    goPackage // package being tested
	TestBinary string    // path to test binary
	Test2JSON  bool      // whether to use test2json
}

// testPackage runs the test binary testBin testing package pkg
// with the provided flags.
func (cmd *mainCmd) testPackage(r testPackageRequest) error {
	// test.gocoverdir is an undocumented flag that tells the test binary
	// where to write coverage data.
	// See https://github.com/golang/go/issues/51430#issuecomment-1344711300
	flags := append(r.Flags, "-test.gocoverdir="+r.CoverDir)

	var testCmd *exec.Cmd
	if r.Test2JSON {
		// Run the test binary through test2json.
		args := []string{
			"tool", "test2json",
			"-t",                 // add timestamps
			"-p", r.Package.Name, // add package name
			r.TestBinary,
		}

		testCmd = exec.Command("go", append(args, flags...)...) //nolint:gosec
	} else {
		testCmd = exec.Command(r.TestBinary, flags...) //nolint:gosec
	}

	testCmd.Stdout = cmd.Stdout
	testCmd.Stderr = cmd.Stderr
	testCmd.Dir = r.Package.Dir // tests always run in the package directory
	testCmd.Env = append(cmd.Environ(), "GOCOVERDIR="+r.CoverDir)

	cmd.debugf("*** %v", testCmd)
	if err := testCmd.Run(); err != nil {
		return fmt.Errorf("run test binary: %w", err)
	}
	return nil
}

// goPackage is a subset of the struct returned by 'go list -json'
// containing only the fields we care about.
type goPackage struct {
	// Name of the package.
	Name string `json:"Name"`

	// Directory containing the package.
	Dir string `json:"Dir"`

	// Import path of the package.
	ImportPath string `json:"ImportPath"`
}

// expandPatterns returns a list of all packages matching the given patterns.
func (cmd *mainCmd) expandPatterns(patterns []string) ([]goPackage, error) {
	// Run 'go list' to find all packages matching the pattern.
	goList := exec.Command("go", append([]string{"list", "-json"}, patterns...)...) //nolint:gosec
	goList.Stderr = cmd.Stderr
	stdout, err := goList.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stdout: %w", err)
	}

	cmd.debugf("*** %v", goList)
	if err := goList.Start(); err != nil {
		return nil, fmt.Errorf("start 'go list': %w", err)
	}

	// Use a map to deduplicate packages
	// on the off chance that patterns are overlapping.
	allPackages := make(map[string]goPackage) // import path -> package

	dec := json.NewDecoder(stdout)
	for dec.More() {
		var pkg goPackage
		if err := dec.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("decode 'go list' output: %w", err)
		}
		allPackages[pkg.ImportPath] = pkg
	}

	if err := goList.Wait(); err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	var packages []goPackage
	for _, pkg := range allPackages {
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool {
		return packages[i].ImportPath < packages[j].ImportPath
	})
	return packages, nil
}
