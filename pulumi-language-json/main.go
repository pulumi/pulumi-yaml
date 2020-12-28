// Copyright 2020, Pulumi Corporation.
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

// pulumi-language-json is the "language host" for Pulumi programs written in JSON. It is responsible for
// evaluating JSON templates, registering resources, outputs, and so on, with the Pulumi engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	pbempty "github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v2/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v2/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v2/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v2/go/common/version"
	"github.com/pulumi/pulumi/sdk/v2/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v2/go/pulumi"
	pulumirpc "github.com/pulumi/pulumi/sdk/v2/proto/go"
	"google.golang.org/grpc"

	"github.com/pulumi/pulumiformation/pulumiformation"
)

// Launches the language host RPC endpoint, which in turn fires up an RPC server implementing the
// LanguageRuntimeServer RPC endpoint.
func main() {
	// Parse the flags and initialize some boilerplate.
	var tracing string
	flag.StringVar(&tracing, "tracing", "", "Emit tracing to a Zipkin-compatible tracing endpoint")
	flag.Parse()
	args := flag.Args()
	logging.InitLogging(false, 0, false)
	cmdutil.InitTracing("pulumi-language-json", "pulumi-language-json", tracing)

	// Fetch the engine address if available so we can do logging, etc.
	var engineAddress string
	if len(args) > 0 {
		engineAddress = args[0]
	}

	// Fire up a gRPC server, letting the kernel choose a free port.
	port, done, err := rpcutil.Serve(0, nil, []func(*grpc.Server) error{
		func(srv *grpc.Server) error {
			host := newLanguageHost(engineAddress, tracing)
			pulumirpc.RegisterLanguageRuntimeServer(srv, host)
			return nil
		},
	}, nil)
	if err != nil {
		cmdutil.Exit(errors.Wrapf(err, "could not start language host RPC server"))
	}

	// Otherwise, print out the port so that the spawner knows how to reach us.
	fmt.Printf("%d\n", port)

	// And finally wait for the server to stop serving.
	if err := <-done; err != nil {
		cmdutil.Exit(errors.Wrapf(err, "language host RPC stopped serving"))
	}
}

// jsonLanguageHost implements the LanguageRuntimeServer interface
// for use as an API endpoint.
type jsonLanguageHost struct {
	engineAddress string
	tracing       string
}

func newLanguageHost(engineAddress, tracing string) pulumirpc.LanguageRuntimeServer {
	return &jsonLanguageHost{
		engineAddress: engineAddress,
		tracing:       tracing,
	}
}

// GetRequiredPlugins computes the complete set of anticipated plugins required by a program.
func (host *jsonLanguageHost) GetRequiredPlugins(ctx context.Context,
	req *pulumirpc.GetRequiredPluginsRequest) (*pulumirpc.GetRequiredPluginsResponse, error) {
	tmpl, err := pulumiformation.Load()
	if err != nil {
		return nil, err
	}
	pkgs := pulumiformation.GetReferencedPackages(&tmpl)
	var plugins []*pulumirpc.PluginDependency
	for _, pkg := range pkgs {
		plugins = append(plugins, &pulumirpc.PluginDependency{
			Kind:    string(workspace.ResourcePlugin),
			Name:    pkg.Package,
			Version: pkg.Version,
			// TODO: Offer a way to specify this either globally or on a per-resource basis
			Server: "",
		})
	}
	return &pulumirpc.GetRequiredPluginsResponse{
		Plugins: plugins,
	}, nil
}

// RPC endpoint for LanguageRuntimeServer::Run. This actually evaluates the JSON-based project.
func (host *jsonLanguageHost) Run(ctx context.Context, req *pulumirpc.RunRequest) (*pulumirpc.RunResponse, error) {
	// Ensure we're in the right directory so files, etc, are in the expected place.
	// TODO: ignoring main for now until we figure out what this means.
	if pwd := req.GetPwd(); pwd != "" {
		os.Chdir(pwd)
	}

	// Use the Pulumi Go SDK to create an execution context and to interact with the engine.
	// This encapsulates a fair bit of the boilerplate otherwise needed to do RPCs, etc.
	pctx, err := pulumi.NewContext(ctx, pulumi.RunInfo{
		Project:     req.GetProject(),
		Stack:       req.GetStack(),
		Config:      req.GetConfig(),
		Parallel:    int(req.GetParallel()),
		DryRun:      req.GetDryRun(),
		MonitorAddr: req.GetMonitorAddress(),
		EngineAddr:  host.engineAddress,
	})
	if err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}
	defer pctx.Close()

	// Now instruct the Pulumi Go SDK to run the pulumformation interpreter.
	if err := pulumi.RunWithContext(pctx, pulumiformation.Run); err != nil {
		return &pulumirpc.RunResponse{Error: err.Error()}, nil
	}

	return &pulumirpc.RunResponse{}, nil
}

func (host *jsonLanguageHost) GetPluginInfo(ctx context.Context, req *pbempty.Empty) (*pulumirpc.PluginInfo, error) {
	return &pulumirpc.PluginInfo{
		Version: version.Version,
	}, nil
}
