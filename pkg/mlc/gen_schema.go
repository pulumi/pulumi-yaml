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
	"context"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

func Schema(decl *ast.TemplateDecl, loader pulumiyaml.PackageLoader) (schema.PackageSpec, error) {
	var pkg schema.PackageSpec
	if decl.Description != nil {
		pkg.Description = decl.Description.Value
	}
	if decl.Name == nil {
		return schema.PackageSpec{}, fmt.Errorf("name not specified on decl")
	}
	pkg.Name = decl.Name.Value

	r := schema.ResourceSpec{
		ObjectTypeSpec: schema.ObjectTypeSpec{
			Description: pkg.Description,
			Properties:  map[string]schema.PropertySpec{},
			Type:        pkg.Name + ":index:Component",
			Required:    []string{},
		},
		InputProperties: map[string]schema.PropertySpec{},
		StateInputs:     &schema.ObjectTypeSpec{},
		IsComponent:     true,
	}

	ctx, err := pulumi.NewContext(context.TODO(), pulumi.RunInfo{
		Project:          decl.Name.Value,
		Stack:            "Component",
		Config:           map[string]string{},
		ConfigSecretKeys: []string{},
		Parallel:         1,
		DryRun:           true,
	})
	if err != nil {
		return schema.PackageSpec{}, fmt.Errorf("could not make context: %w", err)
	}
	ctx.Log = &logEmpty{}

	typing, err := pulumiyaml.TypeDecl(decl, ctx, loader)
	if err != nil {
		return schema.PackageSpec{}, err
	}

	for _, input := range decl.Configuration.Entries {
		k, v := input.Key.Value, input.Value
		typ := typing.TypeConfig(k)
		if typ == nil {
			return schema.PackageSpec{}, fmt.Errorf("configuration type of '%s' unknown", k)
		}
		spec, _, err := typeSpec(typ)
		if err != nil {
			return schema.PackageSpec{}, err
		}
		def := schemaDefaultValue(v.Default)

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
		typ := typing.TypeOutput(k)
		if typ == nil {
			return schema.PackageSpec{}, fmt.Errorf("output type of '%s' unknown", k)
		}
		spec, required, err := typeSpec(typ)
		if err != nil {
			return schema.PackageSpec{}, err
		}

		if required {
			r.Required = append(r.Required, k)
		}

		r.Properties[k] = schema.PropertySpec{
			TypeSpec: spec,
		}
	}

	pkg.Resources = map[string]schema.ResourceSpec{pkg.Name + ":index:Component": r}
	return pkg, nil
}

func typeSpec(t schema.Type) (schema.TypeSpec, bool, error) {
	wrapInner := func(t schema.Type, wrap func(inner schema.TypeSpec) schema.TypeSpec) (schema.TypeSpec, bool, error) {
		inner, required, err := typeSpec(t)
		if err != nil {
			return schema.TypeSpec{}, false, err
		}

		return wrap(inner), required, nil
	}
	switch t := t.(type) {
	case *schema.ArrayType:
		return wrapInner(t.ElementType, func(inner schema.TypeSpec) schema.TypeSpec {
			return schema.TypeSpec{
				Type:  "array",
				Items: &inner,
			}
		})
	case *schema.MapType:
		return wrapInner(t.ElementType, func(inner schema.TypeSpec) schema.TypeSpec {
			return schema.TypeSpec{
				Type:                 "object",
				AdditionalProperties: &inner,
			}
		})
	case *schema.InputType:
		inner, required, err := typeSpec(t.ElementType)
		if err != nil {
			return schema.TypeSpec{}, false, err
		}
		inner.Plain = false
		return inner, required, nil
	case *schema.OptionalType:
		inner, _, err := typeSpec(t.ElementType)
		if err != nil {
			return schema.TypeSpec{}, false, err
		}
		return inner, false, nil

	case *schema.ObjectType:
		if strings.HasPrefix(t.Token, "pulumi:adhock:") {
			// Give the ad hock token a name, and then export it
		} else {
			pkg := t.Package.Name
			if t.Package.Version == nil {
				return schema.TypeSpec{}, false, fmt.Errorf("package '%s' missing version", pkg)
			}
			version := t.Package.Version.String()
			return schema.TypeSpec{
				Ref: fmt.Sprintf("/%s/v%s/schema.json#/types/%s", pkg, version, t.Token),
			}, true, nil
		}
	}
	spec := schema.TypeSpec{
		Plain: true,
	}
	if schema.IsPrimitiveType(t) {
		switch t {
		case schema.BoolType, schema.IntType, schema.NumberType, schema.StringType:
			spec.Type = t.String()
		default:
			spec.Ref = fmt.Sprintf("pulumi.json#/%s", strings.TrimPrefix(t.String(), "pulumi:pulumi:"))
		}
		return spec, true, nil
	}
	return schema.TypeSpec{}, false, fmt.Errorf("cannot handle %[1]s (%[1]T)", t)
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
