// Copyright 2022, Pulumi Corporation.  All rights reserved.

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/dotnet"
	gogen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	"github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	"github.com/pulumi/pulumi/pkg/v3/codegen/python"
	"github.com/spf13/cobra"

	yaml "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/codegen"
	"github.com/pulumi/pulumi-yaml/pkg/version"
)

var yamlPath string
var outPath string

func loadTemplate() (*ast.TemplateDecl, hcl.Diagnostics, error) {
	if yamlPath == "" {
		t, diags, err := yaml.Load()
		return t, hcl.Diagnostics(diags), err
	}
	t, diags, err := yaml.LoadFile(yamlPath)
	return t, hcl.Diagnostics(diags), err
}

var ejectCmd *cobra.Command = &cobra.Command{
	Use:           "eject",
	Short:         "convert Pulumi YAML to Pulumi IL",
	Long:          "convert Pulumi YAML to Pulumi IL",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       version.Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		template, diags, err := loadTemplate()
		if err != nil {
			return err
		}

		defer func() {
			if len(diags) != 0 {
				//nolint:errcheck // in defer, lack a proper recourse for an error here.
				template.NewDiagnosticWriter(os.Stderr, 0, true).WriteDiagnostics(diags)
			}
		}()

		if diags.HasErrors() {
			return diags
		}

		il, cdiags, err := codegen.ConvertTemplateIL(template, nil)
		diags = diags.Extend(cdiags)
		if err != nil {
			return err
		}
		if diags.HasErrors() {
			return diags
		}

		fmt.Printf("%v", il)
		return nil
	}}

func generateCmd(name, friendlyName string, generate codegen.GenerateFunc) *cobra.Command {
	return &cobra.Command{
		Use:           name,
		Short:         "convert Pulumi YAML to " + friendlyName,
		Long:          "convert Pulumi YAML to " + friendlyName,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			template, diags, err := loadTemplate()
			if err != nil {
				return err
			}

			defer func() {
				if len(diags) != 0 {
					//nolint:errcheck // in defer, lack a proper recourse for an error here.
					template.NewDiagnosticWriter(os.Stderr, 0, true).WriteDiagnostics(diags)
				}
			}()

			if diags.HasErrors() {
				return diags
			}

			files, cdiags, err := codegen.ConvertTemplate(template, generate, nil)
			diags = diags.Extend(cdiags)
			if err != nil {
				return err
			}
			if diags.HasErrors() {
				return diags
			}

			for filename, bytes := range files {
				if err := ioutil.WriteFile(filename, bytes, 0600); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func main() {
	cmd := &cobra.Command{
		Use:           "yaml2pulumi",
		Short:         "Convert Pulumi YAML to higher-level languages",
		Long:          "Convert Pulumi YAML to higher-level languages",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVarP(&yamlPath, "file", "f", "", "the path of the YAML file to convert")
	cmd.PersistentFlags().StringVarP(&outPath, "out", "o", "", "the path of the file to write")

	cmd.AddCommand(
		generateCmd("csharp", "C#", dotnet.GenerateProgram),
		generateCmd("go", "Go", gogen.GenerateProgram),
		generateCmd("python", "Python", python.GenerateProgram),
		generateCmd("typescript", "TypeScript", nodejs.GenerateProgram),
		ejectCmd)

	if err := cmd.Execute(); err != nil {
		if _, ok := err.(hcl.Diagnostics); !ok {
			fmt.Fprintf(os.Stderr, "%v\n", err)
		}
		os.Exit(-1)
	}
}
