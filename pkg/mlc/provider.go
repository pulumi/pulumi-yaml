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

package mlc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	providersdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
)

// Serve launches the gRPC server for the resource provider.
func Serve(name string) error {
	path := execPath(name)
	_, decl, err := getDecl(path)
	if err != nil {
		return err
	}
	loader, err := pulumiyaml.NewPackageLoader()
	if err != nil {
		return fmt.Errorf("could not get package loader: %w", err)
	}
	spec, err := Schema(decl, loader)
	if err != nil {
		return fmt.Errorf("could not generate spec: %w", err)
	}
	schema, err := json.Marshal(spec)
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

func getDecl(path string) ([]byte, *ast.TemplateDecl, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	decl, diags, err := pulumiyaml.LoadYAMLBytes(path, bytes)
	if err != nil {
		return nil, nil, err
	}
	if diags.HasErrors() {
		return nil, nil, diags
	}

	if decl.Name == nil || decl.Name.Value == "" {
		return nil, nil, fmt.Errorf("Name needed for MLC")
	}

	return bytes, decl, nil
}

func execPath(name string) string {
	pluginDir := filepath.Join(os.Getenv("HOME"), ".pulumi", "plugins")
	folder := fmt.Sprintf("resource-%s-v1.0.0", name)
	file := fmt.Sprintf("pulumi-resource-%s", name)
	return filepath.Join(pluginDir, folder, file)
}

func findServe(args []string) int {
	for i := 0; i < len(os.Args); i++ {
		if os.Args[i] == "-serve" {
			return i
		}
	}
	return -1
}

// ShouldServe returns true if a the package should serve as a MLC provider.
func ShouldServe(args []string) bool {
	return findServe(args) != -1
}

// Retrieve the value associated with the -serve flag.
func GetAndCleanArgs() (string, error) {
	i := findServe(os.Args)
	if i+1 == len(os.Args) {
		return "", fmt.Errorf("the -serve flag needs an argument")
	}
	serve := os.Args[i+1]
	// Remove the serve flag
	os.Args = append(os.Args[:i], os.Args[i+2:]...)
	// Remove the host location flag flag
	os.Args = append(os.Args[:1], os.Args[2:]...)
	return serve, nil
}
