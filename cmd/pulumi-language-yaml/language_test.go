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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/server"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	pbempty "google.golang.org/protobuf/types/known/emptypb"

	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	testingrpc "github.com/pulumi/pulumi/sdk/v3/proto/go/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type hostEngine struct {
	pulumirpc.UnimplementedEngineServer
	t *testing.T

	logLock         sync.Mutex
	logRepeat       int
	previousMessage string
}

func (e *hostEngine) Log(_ context.Context, req *pulumirpc.LogRequest) (*pbempty.Empty, error) {
	e.logLock.Lock()
	defer e.logLock.Unlock()

	var sev diag.Severity
	switch req.Severity {
	case pulumirpc.LogSeverity_DEBUG:
		sev = diag.Debug
	case pulumirpc.LogSeverity_INFO:
		sev = diag.Info
	case pulumirpc.LogSeverity_WARNING:
		sev = diag.Warning
	case pulumirpc.LogSeverity_ERROR:
		sev = diag.Error
	default:
		return nil, fmt.Errorf("Unrecognized logging severity: %v", req.Severity)
	}

	message := req.Message
	if os.Getenv("PULUMI_LANGUAGE_TEST_SHOW_FULL_OUTPUT") != "true" {
		// Cut down logs so they don't overwhelm the test output
		if len(message) > 1024 {
			message = message[:1024] + "... (truncated, run with PULUMI_LANGUAGE_TEST_SHOW_FULL_OUTPUT=true to see full logs))"
		}
	}

	if e.previousMessage == message {
		e.logRepeat++
		return &pbempty.Empty{}, nil
	}

	if e.logRepeat > 1 {
		e.t.Logf("Last message repeated %d times", e.logRepeat)
	}
	e.logRepeat = 1
	e.previousMessage = message

	if req.StreamId != 0 {
		e.t.Logf("(%d) %s[%s]: %s", req.StreamId, sev, req.Urn, message)
	} else {
		e.t.Logf("%s[%s]: %s", sev, req.Urn, message)
	}
	return &pbempty.Empty{}, nil
}

func runEngine(t *testing.T) string {
	// Run a gRPC server that implements the Pulumi engine RPC interface. But all we do is forward logs on to T.
	engine := &hostEngine{t: t}
	stop := make(chan bool)
	t.Cleanup(func() {
		close(stop)
	})
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: stop,
		Init: func(srv *grpc.Server) error {
			pulumirpc.RegisterEngineServer(srv, engine)
			return nil
		},
		Options: rpcutil.OpenTracingServerInterceptorOptions(nil),
	})
	require.NoError(t, err)
	return fmt.Sprintf("127.0.0.1:%v", handle.Port)
}

func runTestingHost(t *testing.T) (string, testingrpc.LanguageTestClient) {
	// We can't just go run the pulumi-test-language package because of
	// https://github.com/golang/go/issues/39172, so we build it to a temp file then run that.
	binary := t.TempDir() + "/pulumi-test-language"
	cmd := exec.Command("go", "build", "-C", "../../pulumi/cmd/pulumi-test-language", "-o", binary) //nolint:gosec
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

	engineAddress := runEngine(t)
	return engineAddress, client
}

// Add test names here that are expected to fail and the reason why they are failing
var expectedFailures = map[string]string{
	"l1-builtin-can":                        "#721 generation unimplemented",
	"l1-builtin-try":                        "#721 generation unimplemented",
	"l1-config-types":                       "Failed to generate YAML program: *model.BinaryOpExpression; Unimplemented! Needed for  aNumber + 1.25",
	"l1-proxy-index":                        "run bailed",
	"l1-output-null":                        "test failing",
	"l2-component-call-simple":              "#722 generation unimplemented",
	"l2-resource-option-retain-on-delete":   "#723 generation unimplemented",
	"l2-resource-option-deleted-with":       "#724 test failing",
	"l2-resource-parent-inheritance":        "expected parent to be retain on delete",
	"l2-failed-create-continue-on-error":    "#725 test failing",
	"l2-rtti":                               "test failing",
	"l2-provider-call":                      "Traversal not allowed on function result",
	"l2-provider-call-explicit":             "Traversal not allowed on function result",
	"l2-proxy-index":                        "test failing",
	"l2-provider-grpc-config-schema-secret": "Detected a secret leak in state",
	"l2-component-property-deps":            "Traversal not allowed on function result",
	"l3-component-simple":                   "not implemented",
}

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

	tests, err := engine.GetLanguageTests(context.Background(), &testingrpc.GetLanguageTestsRequest{})
	require.NoError(t, err)

	cancel := make(chan bool)

	// Run the language plugin
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Init: func(srv *grpc.Server) error {
			host := server.NewLanguageHost(engineAddress, "", true /* useRPCLoader */)
			pulumirpc.RegisterLanguageRuntimeServer(srv, host)
			return nil
		},
		Cancel: cancel,
	})
	require.NoError(t, err)

	// Create a temp project dir for the test to run in
	rootDir := t.TempDir()

	snapshotDir := "./testdata/"

	// Prepare to run the tests
	prepare, err := engine.PrepareLanguageTests(context.Background(), &testingrpc.PrepareLanguageTestsRequest{
		LanguagePluginName:   "yaml",
		LanguagePluginTarget: fmt.Sprintf("127.0.0.1:%d", handle.Port),
		TemporaryDirectory:   rootDir,
		SnapshotDirectory:    snapshotDir,
	})
	require.NoError(t, err)

	//nolint:paralleltest // YAML runtime is stateful and not safe to run in parallel.
	for _, tt := range tests.Tests {
		t.Run(tt, func(t *testing.T) {
			if expected, ok := expectedFailures[tt]; ok {
				t.Skipf("Skipping known failure: %s", expected)
			}

			result, err := engine.RunLanguageTest(context.Background(), &testingrpc.RunLanguageTestRequest{
				Token: prepare.Token,
				Test:  tt,
			})

			require.NoError(t, err)
			for _, msg := range result.Messages {
				t.Log(msg)
			}
			log(t, "stdout", result.Stdout)
			log(t, "stderr", result.Stderr)
			assert.True(t, result.Success)
		})
	}

	t.Cleanup(func() {
		close(cancel)
		assert.NoError(t, <-handle.Done)
	})
}
