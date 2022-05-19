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
	"fmt"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

func Schema(decl *ast.TemplateDecl) schema.PackageSpec {
	var pkg schema.PackageSpec
	pkg.Description = decl.Description.Value
	pkg.Name = decl.Name.Value

	r := schema.ResourceSpec{
		ObjectTypeSpec: schema.ObjectTypeSpec{
			Description: pkg.Description,
			Properties:  map[string]schema.PropertySpec{},
			Type:        pkg.Name + ":Component",
			Required:    []string{},
		},
		InputProperties: map[string]schema.PropertySpec{},
		StateInputs:     &schema.ObjectTypeSpec{},
		IsComponent:     true,
	}

	for _, input := range decl.Configuration.Entries {
		k, v := input.Key.Value, input.Value
		spec := schema.TypeSpec{}
		def := schemaDefaultValue(v.Default)
		if v.Type != nil {
			switch v.Type.Value {
			case "String":
				spec.Type = "string"
			case "Number":
				spec.Type = "number" //nolint:goconst
			case "Boolean":
				spec.Type = "boolean"
			default:
				switch def.(type) {
				case string:
					spec.Type = "string"
				case float64:
					spec.Type = "number"
				case int:
					spec.Type = "number"
				case bool:
					spec.Type = "boolean"
				default:
					spec.Ref = "pulumi.json#/Any"
				}
			}
		}

		r.InputProperties[k] = schema.PropertySpec{
			TypeSpec: spec,
			Default:  def,
			DefaultInfo: &schema.DefaultSpec{
				Environment: []string{k},
			},
			Secret: v.Secret != nil && v.Secret.Value,
		}
		if def == nil {
			r.RequiredInputs = append(r.Required, k)
		}
	}

	for _, output := range decl.Outputs.Entries {
		k := output.Key.Value
		r.Properties[k] = schema.PropertySpec{
			TypeSpec: schema.TypeSpec{
				// Anything better would require typing the whole program (which
				// we should do eventually)
				Ref: "pulumi.json#/Any",
			},
		}
	}

	pkg.Resources = map[string]schema.ResourceSpec{pkg.Name + ":index:Component": r}
	return pkg
}

func schemaDefaultValue(e ast.Expr) interface{} {
	switch e := e.(type) {
	case *ast.StringExpr:
		return e.Value
	case *ast.NumberExpr:
		return e.Value
	case *ast.BooleanExpr:
		return e.Value
	case nil:
		return nil
	default:
		panic(fmt.Sprintf("Unknown default value: %s", e))
	}
}
