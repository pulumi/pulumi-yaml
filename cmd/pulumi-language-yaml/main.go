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
	"context"
	"os"
	"os/signal"
	"time"

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

	logging.InitLogging(false, 0, false)

	rc, err := rpcCmd.NewRpcCmd(&rpcCmd.RpcCmdConfig{
		TracingName:  "pulumi-language-yaml",
		RootSpanName: "pulumi-language-yaml",
	})
	if err != nil {
		cmdutil.Exit(err)
	}

	var root string
	var compiler string
	rc.Flag.StringVar(&root, "root", "", "Root of the program execution")
	rc.Flag.StringVar(&compiler, "compiler", "", "Compiler to use to pre-process YAML")

	rc.Flag.Parse(os.Args[1:])

	rc.Run(func(srv *grpc.Server) error {
		host := server.NewLanguageHost(rc.EngineAddress, rc.Tracing, compiler)
		pulumirpc.RegisterLanguageRuntimeServer(srv, host)
		return nil
	}, func() {})
}

func setupHealthChecks(engineAddress string) (chan bool, error) {
	// If the health check begins failing or we receive a SIGINT,
	// we'll cancel the context.
	//
	// The returned channel is used to notify the server that it should
	// stop serving and exit.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	// map the context Done channel to the rpcutil boolean cancel channel
	cancelChannel := make(chan bool)
	go func() {
		<-ctx.Done()
		cancel() // deregister the signal handler
		close(cancelChannel)
	}()
	err := rpcutil.Healthcheck(ctx, engineAddress, 5*time.Minute, cancel)

	if err != nil {
		return nil, err
	}
	return cancelChannel, nil
}
