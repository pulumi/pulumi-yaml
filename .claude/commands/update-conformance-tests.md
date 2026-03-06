We are upgrading the version of `github.com/pulumi/pulumi/{pkg,sdk}/v3` used in this
repo. As part of that upgrade, we need to update our local conformance tests.  Run `go
test ./... -run TestLanguage 2>&1` to get a list of failing tests. Add each failing test
to your TODO list.

For each test:

Run the test individually (`PULUMI_ACCEPT=1 go test ./... -run TestLanguage/$TEST-NAME`).

If the test passes, check in the generated files and commit (`git add
cmd/pulumi-language-yaml/testdata/`) with `Add $TEST-NAME` as the commit message.

If the test fails, delete any files running that test generated (`git restore
cmd/pulumi-language-yaml/testdata/ `) & add that test to the
`cmd/pulumi-language-yaml/language_test.go`'s expectedFailures map with summary of the
error message causing it to fail, then commit with `Skip $TEST-NAME`.

Always run `make lint` before each commit.

When you are done with all tests, run `make lint` and then (after all lints pass), run `go
test ./...` again (without PULUMI_ACCEPT=1) to ensure that all tests pass.

---

Rules:

1. Do not push any commits to origin.
1. Do not attempt to fix any detected failures, just skip them. Only skip language tests
   that were added during thee upgrade. Do not skip old language tests that only now
   started failing.
1. If it looks like tests are failing due to setup failures, stop and alert the user.
1. Do not run tests in parallel. Work sequentially across each test.
1. Use a sub-agent for each test to keep context useage tight.
