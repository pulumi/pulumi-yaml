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

// pulumi-converter-yaml is the "language converter" for Pulumi programs written in YAML or JSON. It is
// responsible for translating JSON/YAML templates into PCL.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"

	yamlgen "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/encoding"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/spf13/afero"
	"google.golang.org/grpc"
)

type yamlConverter struct {
}

func (*yamlConverter) Close() error {
	return nil
}

func (*yamlConverter) ConvertState(ctx context.Context,
	req *plugin.ConvertStateRequest,
) (*plugin.ConvertStateResponse, error) {
	return nil, errors.New("not implemented")
}

// writeProgram writes a project and pcl program to the given filesystem
func writeProgram(fs afero.Fs, proj *workspace.Project, program *pcl.Program) error {
	contract.Requiref(fs != nil, "fs", "must not be nil")
	contract.Requiref(proj != nil, "proj", "must not be nil")
	contract.Requiref(program != nil, "program", "must not be nil")

	err := program.WriteSource(fs)
	if err != nil {
		return fmt.Errorf("writing program: %w", err)
	}

	projBytes, err := encoding.YAML.Marshal(proj)
	if err != nil {
		return fmt.Errorf("marshaling project: %w", err)
	}

	err = afero.WriteFile(fs, "Pulumi.yaml", projBytes, 0o644)
	if err != nil {
		return fmt.Errorf("writing project: %w", err)
	}

	return nil
}

func (*yamlConverter) ConvertProgram(ctx context.Context,
	req *plugin.ConvertProgramRequest,
) (*plugin.ConvertProgramResponse, error) {
	loader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, err
	}
	proj, program, err := yamlgen.Eject(req.SourceDirectory, loader)
	if err != nil {
		return nil, fmt.Errorf("load yaml program: %w", err)
	}
	fs := afero.NewBasePathFs(afero.NewOsFs(), req.TargetDirectory)
	err = writeProgram(fs, proj, program)
	if err != nil {
		return nil, fmt.Errorf("write program to intermediate directory: %w", err)
	}

	return &plugin.ConvertProgramResponse{}, nil
}

// Launches the converter RPC endpoint
func main() {
	cancelch := make(chan bool)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	go func() {
		<-ctx.Done()
		cancel() // deregister the interrupt handler
		close(cancelch)
	}()

	// Fire up a gRPC server, letting the kernel choose a free port for us.
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancelch,
		Init: func(srv *grpc.Server) error {
			pulumirpc.RegisterConverterServer(srv, plugin.NewConverterServer(&yamlConverter{}))
			return nil
		},
		Options: rpcutil.OpenTracingServerInterceptorOptions(nil),
	})
	if err != nil {
		log.Fatalf("fatal: %v", err)
	}

	// The converter protocol requires that we now write out the port we have chosen to listen on.
	fmt.Printf("%d\n", handle.Port)

	// Finally, wait for the server to stop serving.
	if err := <-handle.Done; err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
