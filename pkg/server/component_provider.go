// Copyright 2025, Pulumi Corporation.
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

package server

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type componentProvider struct {
	pulumirpc.UnimplementedResourceProviderServer

	host      *grpc.ClientConn
	name      string
	schema    []byte
	construct provider.ConstructFunc
}

// GetPluginInfo returns generic information about this plugin, like its version.
func (p *componentProvider) GetPluginInfo(context.Context, *emptypb.Empty) (*pulumirpc.PluginInfo, error) {
	// We fill in the version on the engine side for components.
	return &pulumirpc.PluginInfo{
		Version: "0.0.0",
	}, nil
}

// GetSchema returns the JSON-encoded schema for this provider's package.
func (p *componentProvider) GetSchema(ctx context.Context,
	req *pulumirpc.GetSchemaRequest,
) (*pulumirpc.GetSchemaResponse, error) {
	if v := req.GetVersion(); v != 0 {
		return nil, fmt.Errorf("unsupported schema version %d", v)
	}
	return &pulumirpc.GetSchemaResponse{Schema: string(p.schema)}, nil
}

// Configure configures the resource provider with "globals" that control its behavior.
func (p *componentProvider) Configure(ctx context.Context,
	req *pulumirpc.ConfigureRequest,
) (*pulumirpc.ConfigureResponse, error) {
	return &pulumirpc.ConfigureResponse{
		AcceptSecrets:   true,
		SupportsPreview: true,
		AcceptResources: true,
		AcceptOutputs:   true,
	}, nil
}

// Construct creates a new instance of the provided component resource and returns its state.
func (p *componentProvider) Construct(ctx context.Context,
	req *pulumirpc.ConstructRequest,
) (*pulumirpc.ConstructResponse, error) {
	return provider.Construct(ctx, req, p.host, p.construct)
}

// Call dynamically executes a method in the provider associated with a component resource.
func (p *componentProvider) Call(ctx context.Context,
	req *pulumirpc.CallRequest,
) (*pulumirpc.CallResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Call is not yet implemented")
}

// Cancel signals the provider to gracefully shut down and abort any ongoing resource operations.
// Operations aborted in this way will return an error (e.g., `Update` and `Create` will either a
// creation error or an initialization error). Since Cancel is advisory and non-blocking, it is up
// to the host to decide how long to wait after Cancel is called before (e.g.)
// hard-closing any gRPC connection.
func (p *componentProvider) Cancel(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// Attach attaches to the engine for an already running provider.
func (p *componentProvider) Attach(ctx context.Context,
	req *pulumirpc.PluginAttach,
) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "Attach is not yet implemented")
}

// GetMapping fetches the conversion mapping (if any) for this resource provider.
func (p *componentProvider) GetMapping(ctx context.Context,
	req *pulumirpc.GetMappingRequest,
) (*pulumirpc.GetMappingResponse, error) {
	return &pulumirpc.GetMappingResponse{Provider: "", Data: nil}, nil
}
