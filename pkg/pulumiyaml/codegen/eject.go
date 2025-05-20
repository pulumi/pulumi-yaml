// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

var ProjectKeysToOmit = []string{"configuration", "resources", "outputs", "variables"}

// Eject on a YAML program directory returns a Pulumi Project and a YAML program which has been
// parsed and converted to the intermediate PCL language
func Eject(dir string, loader schema.ReferenceLoader) (*workspace.Project, *pcl.Program, error) {
	// `*pcl.Program`'s maintains an internal reference to the loader that was used during
	// its creation. This means the lifetime of the returned program is tied to the
	// lifetime of the loader passed to EjectProgram and ultimately to the host that
	// created that loader.
	//
	// To avoid panics (see https://github.com/pulumi/pulumi/issues/10875), we make it the
	// caller's responsibility to cleanup the loader used. This prevents us from providing
	// a default loader, since there would be no way to clean it up correctly.
	if loader == nil {
		panic("must provide a non-nil loader")
	}
	proj, template, diags, err := LoadTemplate(dir)
	if err != nil {
		return nil, nil, err
	}
	if template == nil && diags.HasErrors() {
		return nil, nil, fmt.Errorf("failed to load the template: %s", diags.Error())
	}
	// remove extraneous keys from Pulumi.yaml project file
	if proj.AdditionalKeys != nil {
		for _, k := range ProjectKeysToOmit {
			delete(proj.AdditionalKeys, k)
		}
	}
	diagWriter := template.NewDiagnosticWriter(os.Stderr, 0, true)
	if len(diags) != 0 {
		err := diagWriter.WriteDiagnostics(diags)
		if err != nil {
			return nil, nil, err
		}
	}

	program, pdiags, err := EjectProgram(template, loader)
	if len(diags) != 0 {
		err := diagWriter.WriteDiagnostics(pdiags)
		if err != nil {
			return nil, nil, err
		}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load YAML program, %v", err)
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

func loadProjectFromDir(dir string) (*workspace.Project, string, error) {
	// walk up the directory to find the project file path i.e. path to Pulumi.yaml
	projectPath, err := getProjectPath(dir)
	if err != nil {
		return nil, "", fmt.Errorf("finding project path %w", err)
	} else if projectPath == "" {
		return nil, "", fmt.Errorf(
			"no Pulumi.yaml project file found (searching upwards from %s)", dir)
	}

	// parse project info from its path
	proj, err := workspace.LoadProject(projectPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load Pulumi project located at %q: %w", projectPath, err)
	}

	// find the working directory of the project (parent of the project file)
	projectDir := filepath.Dir(projectPath)

	if proj.Main == "" {
		return proj, projectDir, nil
	}

	// the project specifies a main directory relative to the project directory itself
	main := path.Join(projectDir, proj.Main)
	return proj, main, nil
}

func LoadTemplate(dir string) (*workspace.Project, *ast.TemplateDecl, hcl.Diagnostics, error) {
	proj, main, err := loadProjectFromDir(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	var t *ast.TemplateDecl
	var diags syntax.Diagnostics
	compilerOpt, useCompiler := proj.Runtime.Options()["compiler"]
	if useCompiler {
		compiler, ok := compilerOpt.(string)
		if !ok {
			return nil, nil, nil, fmt.Errorf("compiler option must be a string, got %v", reflect.TypeOf(compilerOpt))
		}
		t, diags, err = pulumiyaml.LoadFromCompiler(compiler, main, nil)
	} else {
		t, diags, err = pulumiyaml.LoadDir(main)
	}

	// unset this, as we've already parsed the YAML program in "main" and it won't be valid for convert
	cleanedProj := *proj
	cleanedProj.Main = ""
	return &cleanedProj, t, diags.HCL(), err
}
