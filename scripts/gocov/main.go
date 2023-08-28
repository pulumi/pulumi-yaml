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
		p.TestFlags = append(p.TestFlags, "-test.v")
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
	coverPkgsArg := strings.Join(coverPkgs, ",")

	// Build the test binaries with coverage tracking.
	testBinaries := make([]string, len(packages))
	for i, pkg := range packages {
		testBinaries[i], err = cmd.buildPackage(binDir, coverPkgsArg, pkg)
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

		if err := cmd.testPackage(p.CoverDir, p.TestFlags, pkg, testBin); err != nil {
			errs[i] = fmt.Errorf("test package %v: %w", pkg.ImportPath, err)
		}
	}
	return errors.Join(errs...)
}

// buildPackage builds the given package and returns the path to its test binary.
// Returns an empty path if the package has no tests.
func (cmd *mainCmd) buildPackage(binDir, coverpkg string, pkg goPackage) (string, error) {
	// Test binaries are typically named $pkg.test after the package name.
	// Since we're placing them all in the same directory,
	// we'll want to ensure there are no name collisions.
	// We can use os.CreateTemp to generate unique names.
	var binPath string
	{
		f, err := os.CreateTemp(binDir, pkg.Name+".test")
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

	// Build the test binary with coverage instrumentation.
	buildCmd := exec.Command("go", "test", "-c", //nolint:gosec
		"-cover",
		"-coverpkg="+coverpkg,
		"-o", binPath,
		pkg.ImportPath,
	)
	buildCmd.Stdout = cmd.Stdout
	buildCmd.Stderr = cmd.Stderr
	buildCmd.Dir = pkg.Dir
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

// testPackage runs the test binary testBin testing package pkg
// with the provided flags.
func (cmd *mainCmd) testPackage(
	coverDir string,
	flags []string,
	pkg goPackage,
	testBin string,
) error {
	// test.gocoverdir is an undocumented flag that tells the test binary
	// where to write coverage data.
	// See https://github.com/golang/go/issues/51430#issuecomment-1344711300
	flags = append(flags, "-test.gocoverdir="+coverDir)

	testCmd := exec.Command(testBin, flags...)
	testCmd.Stdout = cmd.Stdout
	testCmd.Stderr = cmd.Stderr
	testCmd.Dir = pkg.Dir // tests always run in the package directory
	testCmd.Env = append(cmd.Environ(), "GOCOVERDIR="+coverDir)

	cmd.debugf("*** %v", testCmd.Args)
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
