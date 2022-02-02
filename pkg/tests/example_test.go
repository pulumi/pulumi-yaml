package tests

import (
	"os"
	"path/filepath"
	"testing"
)

var awsConfig = StackConfig{map[string]string{
	"aws:region": "us-east",
	"org":        os.Getenv("PULUMI_TEST_OWNER"),
}}

func exampleDir(dir string) string {
	return filepath.Join("../../examples/", dir)
}

func TestExampleGettingStarted(t *testing.T) {
	testWrapper(t, exampleDir("getting-started"), awsConfig)
}

func TestExampleStackreference(t *testing.T) {
	// Stack references only work in a live test, need to exercise the API
	testWrapper(t, exampleDir("stackreference"), awsConfig, RequireLiveRun{})
}

func TestExampleWebserver(t *testing.T) {
	x := exampleDir("webserver")
	testWrapper(t, x, awsConfig)
}

func TestExampleWebserverInvokeJson(t *testing.T) {
	t.Skip("TODO: Invoke syntax.")
	testWrapper(t, exampleDir("webserver-invoke-json"), awsConfig)
}

func TestExampleWebserverInvoke(t *testing.T) {
	t.Skip("TODO: Invoke syntax.")
	testWrapper(t, exampleDir("webserver-invoke"), awsConfig)
}
