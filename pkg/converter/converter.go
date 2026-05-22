// Copyright 2026, Pulumi Corporation.
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

package converter

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	yamlgen "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/encoding"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	codegenrpc "github.com/pulumi/pulumi/sdk/v3/proto/go/codegen"
	"github.com/spf13/afero"
)

func New() plugin.Converter {
	return &converter{}
}

type converter struct{}

func (*converter) Close() error {
	return nil
}

func (*converter) ConvertState(ctx context.Context,
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

// ConvertSnippet converts a YAML mapping describing the inputs to the resource, function, or
// provider identified by req.Token into a PCL snippet. Each entry of req.Attributes is parsed as
// its own YAML expression and rendered as a PCL expression keyed by attribute name, so callers can
// pass template interpolations, fn::invoke calls, or structured values just like they would inside
// a snippet body.
func (*converter) ConvertSnippet(ctx context.Context,
	req *plugin.ConvertSnippetRequest,
) (*plugin.ConvertSnippetResponse, error) {
	if req.Token == "" {
		return nil, errors.New("ConvertSnippet: token is required")
	}

	loader, err := schema.NewLoaderClient(req.TargetLoader)
	if err != nil {
		return nil, err
	}
	defer contract.IgnoreClose(loader)

	pkgLoader := pulumiyaml.NewPackageLoaderFromSchemaLoader(schema.NewCachedLoader(loader))
	defer pkgLoader.Close()

	descriptor, err := packageDescriptor(req.Package)
	if err != nil {
		return nil, err
	}
	pkg, err := pkgLoader.LoadPackage(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("loading package %q: %w", descriptor.PackageName(), err)
	}

	body, bodyDiags, err := yamlgen.ImportSnippet(ctx, req.Token, req.Filename, req.Source, pkg, pkgLoader)
	if err != nil {
		return nil, err
	}

	attrs, attrDiags, err := yamlgen.ImportSnippetAttributes(ctx, req.Attributes, req.Token, pkg, pkgLoader)
	if err != nil {
		return nil, err
	}

	diags := append(bodyDiags.HCL(), attrDiags.HCL()...)
	if bodyDiags.HasErrors() || attrDiags.HasErrors() {
		return &plugin.ConvertSnippetResponse{Diagnostics: diags}, nil
	}

	outFilename := strings.TrimSuffix(req.Filename, filepath.Ext(req.Filename)) + ".pp"
	return &plugin.ConvertSnippetResponse{
		Filename:    outFilename,
		Source:      []byte(fmt.Sprintf("%v", body)),
		Diagnostics: diags,
		Attributes:  attrs,
	}, nil
}

// packageDescriptor builds a schema.PackageDescriptor from a ConvertSnippet request.
func packageDescriptor(req *codegenrpc.GetSchemaRequest) (*schema.PackageDescriptor, error) {
	if req == nil {
		return nil, errors.New("package descriptor is required")
	}

	desc := &schema.PackageDescriptor{
		Name:        req.Package,
		DownloadURL: req.DownloadUrl,
	}
	if req.Version != "" {
		v, err := semver.Parse(req.Version)
		if err != nil {
			return nil, fmt.Errorf("parsing package version %q: %w", req.Version, err)
		}
		desc.Version = &v
	}
	if p := req.Parameterization; p != nil {
		pv, err := semver.Parse(p.Version)
		if err != nil {
			return nil, fmt.Errorf("parsing parameterization version %q: %w", p.Version, err)
		}
		desc.Parameterization = &schema.ParameterizationDescriptor{
			Name:    p.Name,
			Version: pv,
			Value:   p.Value,
		}
	}
	return desc, nil
}

func (*converter) ConvertProgram(ctx context.Context,
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
