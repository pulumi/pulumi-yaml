package mlc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	providersdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
)

// Install by sticking the file in the normal ~/.pulumi/plugins
// With #!/bin/env -S pulumi-language-yaml -serve ${NAME} at the top

// Serve launches the gRPC server for the resource provider.
func Serve(path string) error {
	// Start gRPC service.

	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	decl, diags, err := pulumiyaml.LoadYAMLBytes(path, bytes)
	if err != nil {
		return err
	}
	if diags.HasErrors() {
		return diags
	}

	if decl.Name == nil || decl.Name.Value == "" {
		return fmt.Errorf("Name needed for MLC")
	}

	schema, err := json.Marshal(Schema(decl))
	if err != nil {
		return err
	}

	loader, err := pulumiyaml.NewPackageLoader()
	if err != nil {
		return err
	}

	err = provider.ComponentMain(decl.Name.Value, "1.0.0", schema,
		func(ctx *pulumi.Context, typ, name string, inputs providersdk.ConstructInputs,
			options pulumi.ResourceOption) (*providersdk.ConstructResult, error) {
			if typ == decl.Name.Value+":index:Component" {
				m, err := inputs.Map()
				if err != nil {
					return nil, err
				}
				urn, state, err := pulumiyaml.RunComponentTemplate(ctx, typ, name, options, decl, m, loader)
				if err != nil {
					return nil, err
				}
				return &providersdk.ConstructResult{
					URN:   urn,
					State: state,
				}, nil
			}
			return nil, fmt.Errorf("unknown resource type %s", typ)
		})

	return err
}
