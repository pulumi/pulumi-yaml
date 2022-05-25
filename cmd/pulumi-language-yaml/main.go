// Copyright 2022, Pulumi Corporation.
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

// pulumi-language-yaml is the "language host" for Pulumi programs written in YAML or JSON. It is responsible for
// evaluating JSON/YAML templates, registering resources, outputs, and so on, with the Pulumi engine.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-yaml/pkg/mlc"
	"github.com/pulumi/pulumi-yaml/pkg/server"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
)

// Launches the language host RPC endpoint, which in turn fires up an RPC server implementing the
// LanguageRuntimeServer RPC endpoint.
func main() {
	var serve string
	flag.StringVar(&serve, "serve", "", "Host a Pulumi YAML program as a Pulumi Component Provider")
	// We need to do this before interacting with the flags since that alters
	// global state, and mlc.Serve also calls flag.Parse().
	if mlc.ShouldServe(os.Args) {
		serve, err := mlc.GetAndCleanArgs()
		if err != nil {
			cmdutil.Exit(err)
		}
		if err := mlc.Serve(serve); err != nil {
			cmdutil.Exit(err)
		}
		return
	}

	// Parse the flags and initialize some boilerplate.
	var tracing string
	var root string
	var compiler string
	var install string
	flag.StringVar(&tracing, "tracing", "", "Emit tracing to a Zipkin-compatible tracing endpoint")
	flag.StringVar(&root, "root", "", "Root of the program execution")
	flag.StringVar(&compiler, "compiler", "", "Compiler to use to pre-process YAML")
	flag.StringVar(&install, "install", "", "Install the file as a component resource plugin")
	flag.Parse()

	if install != "" {
		err := mlc.InstallPlugin(install)
		if err != nil {
			cmdutil.Exit(err)
		}
		return
	}

	args := flag.Args()
	logging.InitLogging(false, 0, false)
	cmdutil.InitTracing("pulumi-language-yaml", "pulumi-language-yaml", tracing)

	// Fetch the engine address if available so we can do logging, etc.
	var engineAddress string
	if len(args) > 0 {
		engineAddress = args[0]
	}

	// Fire up a gRPC server, letting the kernel choose a free port.
	port, done, err := rpcutil.Serve(0, nil, []func(*grpc.Server) error{
		func(srv *grpc.Server) error {
			host := server.NewLanguageHost(engineAddress, tracing, compiler)
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
