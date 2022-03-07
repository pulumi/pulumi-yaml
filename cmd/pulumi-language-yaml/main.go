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
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi-yaml/pkg/server"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
)

// Launches the language host RPC endpoint, which in turn fires up an RPC server implementing the
// LanguageRuntimeServer RPC endpoint.
func main() {
	// Parse the flags and initialize some boilerplate.
	var (
		tracing string
		root    string
		convert string
	)
	flag.StringVar(&tracing, "tracing", "", "Emit tracing to a Zipkin-compatible tracing endpoint")
	flag.StringVar(&root, "root", "", "Root of the program execition")
	flag.StringVar(&convert, "convert", "", "The file to convert pcl -> YAML")
	flag.Parse()
	args := flag.Args()
	logging.InitLogging(false, 0, false)
	cmdutil.InitTracing("pulumi-language-yaml", "pulumi-language-yaml", tracing)

	if convert != "" {
		f, err := ioutil.ReadFile(convert)
		if err != nil {
			cmdutil.Exit(err)
		}
		parser := syntax.NewParser()
		err = parser.ParseFile(bytes.NewReader(f), convert)
		if err != nil {
			cmdutil.Exit(fmt.Errorf("Failed to parse pcl: %w", err))
		}
		if parser.Diagnostics.HasErrors() {
			cmdutil.Exit(parser.Diagnostics)
		}

		program, diags, err := pcl.BindProgram(parser.Files,
			pcl.AllowMissingProperties,
			pcl.AllowMissingVariables,
			pcl.SkipResourceTypechecking)
		if err != nil {
			cmdutil.Exit(fmt.Errorf("could not bind program: %w", err))
		}
		if diags.HasErrors() {
			cmdutil.Exit(fmt.Errorf("failed to bind program: %w", diags))
		}

		yaml, diags, err := codegen.GenerateProgram(program)
		if err != nil {
			cmdutil.Exit(fmt.Errorf("could not generate program: %w", err))
		}
		if diags.HasErrors() {
			stderr := os.Stderr
			for _, e := range diags {
				fmt.Fprintf(stderr, "%s\n", e.Error())
			}
			cmdutil.Exit(fmt.Errorf("failed to generate program"))
		}
		for k, v := range yaml {
			fmt.Printf("File: %s\n---\n%s\n...\n", k, string(v))
		}

		return
	}

	// Fetch the engine address if available so we can do logging, etc.
	var engineAddress string
	if len(args) > 0 {
		engineAddress = args[0]
	}

	// Fire up a gRPC server, letting the kernel choose a free port.
	port, done, err := rpcutil.Serve(0, nil, []func(*grpc.Server) error{
		func(srv *grpc.Server) error {
			host := server.NewLanguageHost(engineAddress, tracing)
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
