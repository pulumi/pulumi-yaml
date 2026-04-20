// Copyright 2024, Pulumi Corporation.
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

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/converter"
	"github.com/pulumi/pulumi-yaml/pkg/server"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	testingrpc "github.com/pulumi/pulumi/sdk/v3/proto/go/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runTestingHost(t *testing.T) (string, testingrpc.LanguageTestClient) {
	// We can't just go run the pulumi-test-language package because of
	// https://github.com/golang/go/issues/39172, so we build it to a temp file then run that.
	binary := t.TempDir() + "/pulumi-test-language"
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary,
		"github.com/pulumi/pulumi/pkg/v3/testing/pulumi-test-language") //nolint:gosec
	output, err := cmd.CombinedOutput()
	t.Logf("build output: %s", output)
	require.NoError(t, err)

	cmd = exec.Command(binary)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	stderrReader := bufio.NewReader(stderr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			text, err := stderrReader.ReadString('\n')
			if err != nil {
				wg.Done()
				return
			}
			t.Logf("engine: %s", text)
		}
	}()

	err = cmd.Start()
	require.NoError(t, err)

	stdoutBytes, err := io.ReadAll(stdout)
	require.NoError(t, err)

	address := string(stdoutBytes)

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(rpcutil.OpenTracingClientInterceptor()),
		grpc.WithStreamInterceptor(rpcutil.OpenTracingStreamClientInterceptor()),
		rpcutil.GrpcChannelOptions(),
	)
	require.NoError(t, err)

	client := testingrpc.NewLanguageTestClient(conn)

	t.Cleanup(func() {
		assert.NoError(t, cmd.Process.Kill())
		wg.Wait()
		// We expect this to error because we just killed it.
		contract.IgnoreError(cmd.Wait())
	})

	return address, client
}

// Add test names here that are expected to fail and the reason why they are failing
var expectedFailures = map[string]string{
	"l1-builtin-can":                 "#721 generation unimplemented",
	"l1-builtin-file":                "Unknown Function; YAML does not support fn::filebase64",
	"l1-builtin-sha1":                "Unknown Function; YAML does not support fn::sha1",
	"l1-builtin-list":                "Unknown Function; YAML does not support fn::length",
	"l1-builtin-object":              "Unknown Function; YAML does not support fn::entries",
	"l1-builtin-secret":              "Unknown Function; YAML does not support fn::unsecret",
	"l1-builtin-try":                 "#721 generation unimplemented",
	"l1-config-secret":               "*model.BinaryOpExpression; Unimplemented! Needed for  aNumber + 1.25",
	"l1-config-types-object":         "not yet implemented",
	"l1-config-types-primitive":      "not yet implemented",
	"l2-builtin-object":              "Unknown Function; YAML does not support fn::entries",
	"l2-component-call-simple":       "#722 generation unimplemented",
	"l2-component-property-deps":     "Traversal not allowed on function result",
	"l2-module-format":               "https://github.com/pulumi/pulumi-yaml/issues/951",
	"l2-provider-call":               "Traversal not allowed on function result",
	"l2-provider-call-explicit":      "Traversal not allowed on function result",
	"l2-resource-elide-unknowns":     `*model.BinaryOpExpression; Unimplemented! Needed for  unknown.output == "hello"`,
	"l2-resource-name-type":          "Unknown Function; YAML does not support fn::pulumiResourceName",
	"l2-resource-optional":           "*model.ConditionalExpression; Unimplemented! YAML does not support conditional expressions",
	"l2-resource-primitive-defaults": "missing required property boolean: YAML runtime does not apply primitive defaults",
	"l2-resource-config-objects":     "unrecognized type 'map(bool)' for config variable; undefined variable plainBooleanMap",
	"l2-resource-config-primitives":  "*model.BinaryOpExpression; Unimplemented! Needed for  plainNumber + 0.5",
	"l2-snake-names":                 "not handled correctly",
}

// Add test names here that are expected to fail the converter (eject) round-trip test.
var expectedEjectFailures = map[string]string{}

func log(t *testing.T, name, message string) {
	if os.Getenv("PULUMI_LANGUAGE_TEST_SHOW_FULL_OUTPUT") != "true" {
		// Cut down logs so they don't overwhelm the test output
		if len(message) > 1024 {
			message = message[:1024] + "... (truncated, run with PULUMI_LANGUAGE_TEST_SHOW_FULL_OUTPUT=true to see full logs))"
		}
	}
	t.Logf("%s: %s", name, message)
}

func TestLanguage(t *testing.T) {
	t.Parallel()

	engineAddress, engine := runTestingHost(t)

	tests, err := engine.GetLanguageTests(t.Context(), &testingrpc.GetLanguageTestsRequest{})
	require.NoError(t, err)

	cancel := make(chan bool)
	// Run the language plugin
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Init: func(srv *grpc.Server) error {
			pulumirpc.RegisterLanguageRuntimeServer(srv, server.NewLanguageHost(engineAddress, "", true /* useRPCLoader */))
			pulumirpc.RegisterConverterServer(srv, plugin.NewConverterServer(converter.New()))
			return nil
		},
		Cancel: cancel,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		close(cancel)
		assert.NoError(t, <-handle.Done)
	})

	// Create a temp project dir for the test to run in
	rootDir := t.TempDir()

	snapshotDir := "./testdata/"

	// Prepare to run the tests
	prepare, err := engine.PrepareLanguageTests(t.Context(), &testingrpc.PrepareLanguageTestsRequest{
		LanguagePluginName:    "yaml",
		LanguagePluginTarget:  fmt.Sprintf("127.0.0.1:%d", handle.Port),
		TemporaryDirectory:    rootDir,
		SnapshotDirectory:     snapshotDir,
		ConverterPluginTarget: fmt.Sprintf("127.0.0.1:%d", handle.Port),
	})
	require.NoError(t, err)

	//nolint:paralleltest // YAML runtime is stateful and not safe to run in parallel.
	for _, tt := range tests.Tests {
		t.Run(tt, func(t *testing.T) {
			if strings.HasPrefix(tt, "l3-") {
				t.Skip("YAML does not support level three tests")
			}
			if strings.HasPrefix(tt, "policy-") {
				t.Skip("YAML does not support policy tests")
			}
			if strings.HasPrefix(tt, "provider-") {
				t.Skip("YAML does not support provider tests")
			}

			if expected, ok := expectedFailures[tt]; ok {
				t.Skipf("Skipping known failure: %s", expected)
			}

			result, err := engine.RunLanguageTest(t.Context(), &testingrpc.RunLanguageTestRequest{
				Token:            prepare.Token,
				Test:             tt,
				SkipConvertTests: has(expectedEjectFailures, tt),
			}, grpc.MaxCallRecvMsgSize(1024*1024*1024))

			require.NoError(t, err)
			for _, msg := range result.Messages {
				t.Log(msg)
			}
			log(t, "stdout", result.Stdout)
			log(t, "stderr", result.Stderr)
			assert.True(t, result.Success)
		})
	}
}

func has[K comparable, V any, M ~map[K]V](m M, k K) bool { _, ok := m[k]; return ok }
