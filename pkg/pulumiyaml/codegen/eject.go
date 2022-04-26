// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// Eject on a YAML program directory returns a Pulumi Project and a YAML program which has been
// parsed and converted to the intermediate PCL language
//
// If no loader is provided (nil argument), a new plugin host will be spawned to obtain resource
// provider schemas.
func Eject(dir string, loader schema.Loader) (*workspace.Project, *pcl.Program, error) {
	projectPath, err := getProjectPath(dir)
	if err != nil {
		return nil, nil, err

	} else if projectPath == "" {
		return nil, nil, fmt.Errorf(
			"no Pulumi.yaml project file found (searching upwards from %s). If you have not "+
				"created a project yet, use `pulumi new` to do so", dir)
	}
	projectDir := path.Dir(projectPath)

	proj, err := workspace.LoadProject(projectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load Pulumi project located at %q: %w", projectDir, err)
	}

	template, diags, err := pulumiyaml.LoadDir(projectDir, true)
	diagWriter := template.NewDiagnosticWriter(os.Stderr, 0, true)
	if len(diags) != 0 {
		err := diagWriter.WriteDiagnostics(hcl.Diagnostics(diags))
		if err != nil {
			return nil, nil, err
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load YAML program")
	}

	if loader == nil {
		host, err := newPluginHost()
		if err != nil {
			return nil, nil, err
		}
		loader = schema.NewPluginLoader(host)
		defer host.Close()
	}

	program, pdiags, err := EjectProgram(template, loader)
	if len(diags) != 0 {
		err := diagWriter.WriteDiagnostics(pdiags)
		if err != nil {
			return nil, nil, err
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load YAML program")
	}

	return proj, program, nil
}

func getProjectPath(dir string) (string, error) {
	path, err := workspace.DetectProjectPathFrom(dir)
	if err != nil {
		return "", fmt.Errorf("failed to find current Pulumi project because of "+
			"an error when searching for the Pulumi.yaml file (searching upwards from %s)"+": %w", dir, err)

	} else if path == "" {
		return "", fmt.Errorf(
			"no Pulumi.yaml project file found (searching upwards from %s). If you have not "+
				"created a project yet, use `pulumi new` to do so", dir)
	}

	return path, nil
}
