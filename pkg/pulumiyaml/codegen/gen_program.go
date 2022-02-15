// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

func GenerateProgram(p *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	t, d, err := GenerateTemplate(p)
	if err != nil {
		return nil, nil, err
	}
	bYAML, err := yaml.Marshal(t)
	if err != nil {
		return nil, nil, err
	}
	return map[string][]byte{"Main.yaml": bYAML}, d, nil
}

// Generate a serializable YAML template.
func GenerateTemplate(program *pcl.Program) (pulumiyaml.Template, hcl.Diagnostics, error) {
	// This is not strictly necessary, but will lead to more human seeming generated code.
	nodes := pcl.Linearize(program)
	g := generator{}

	for _, n := range nodes {
		g.genNode(n)
	}

	return g.result, g.diags, nil
}

type generator struct {
	result pulumiyaml.Template
	diags  hcl.Diagnostics
	errs   multierror.Error
}

func (g *generator) genNode(n pcl.Node) {
	switch n := n.(type) {
	case *pcl.Resource:
		g.genResource(n)
	case *pcl.ConfigVariable:
		g.genConfigVariable(n)
	case *pcl.LocalVariable:
		g.genLocalVariable(n)
	case *pcl.OutputVariable:
		g.genOutputVariable(n)
	default:
		panic("Not implemented yet")
	}
}

func (g *generator) genResource(n *pcl.Resource) {
	var provider, version, parent string
	if opts := n.Options; opts != nil {
		provider = g.expr(opts.Provider).(string)
		parent = g.expr(opts.Parent).(string)
	}
	properties := map[string]interface{}{}
	for _, input := range n.Inputs {
		properties[input.Name] = g.expr(input.Value)
	}
	r := &pulumiyaml.Resource{
		Type:                    n.Token,
		Component:               n.Schema.IsComponent,
		Properties:              properties,
		AdditionalSecretOutputs: nil,
		Aliases:                 nil,
		CustomTimeouts:          nil,
		DeleteBeforeReplace:     false,
		DependsOn:               []string{},
		IgnoreChanges:           []string{},
		Import:                  "",
		Parent:                  parent,
		Protect:                 false,
		Provider:                provider,
		Version:                 version,
		Condition:               "",
		Metadata:                nil,
	}

	if g.result.Resources == nil {
		g.result.Resources = map[string]*pulumiyaml.Resource{}
	}
	g.result.Resources[n.Name()] = r
}

func (g *generator) genOutputVariable(n *pcl.OutputVariable) {
	if g.result.Outputs == nil {
		g.result.Outputs = map[string]interface{}{}
	}
	g.result.Outputs[n.Name()] = g.expr(n.Value)
}

func (g *generator) expr(e model.Expression) interface{} {
	switch e := e.(type) {
	case *model.LiteralValueExpression:
		switch e.Type() {
		case model.StringType:
			v := e.Value.AsString()
			return v
		case model.NumberType:
			v := e.Value.AsBigFloat()
			if v.IsInt() {
				i, _ := v.Int64()
				return i
			} else {
				f, _ := v.Float64()
				return f
			}
		}
		return fmt.Sprintf("%.v", e.Value)
	case *model.FunctionCallExpression:
		return g.function(e)
	case *model.ScopeTraversalExpression:
		return "${" + strings.TrimSpace(fmt.Sprintf("%.v", e)) + "}"
	case *model.TemplateExpression:
		s := ""
		for _, expr := range e.Parts {
			if lit, ok := expr.(*model.LiteralValueExpression); ok && model.StringType.AssignableFrom(lit.Type()) {
				s += lit.Value.AsString()
			} else {
				s += fmt.Sprintf("${%.v}", expr)
			}
		}
		return s
	default:
		panic(fmt.Sprintf("Unimplimented: %T", e))
	}
}

func (g *generator) genConfigVariable(n *pcl.ConfigVariable) {
	var defValue interface{}
	typ := n.Type()
	if n.DefaultValue != nil {
		defValue = g.expr(n.DefaultValue)
	}
	config := &pulumiyaml.Configuration{
		Type:                  typ.String(),
		Default:               defValue,
		Secret:                nil,
		AllowedPattern:        nil,
		AllowedValues:         nil,
		ConstraintDescription: "",
		Description:           "",
		MaxLength:             nil,
		MaxValue:              nil,
		MinLength:             nil,
		MinValue:              nil,
	}
	if g.result.Configuration == nil {
		g.result.Configuration = map[string]*pulumiyaml.Configuration{}
	}
	g.result.Configuration[n.Name()] = config
}

func (g *generator) genLocalVariable(n *pcl.LocalVariable) {
	if v := g.result.Variables; v == nil {
		g.result.Variables = map[string]interface{}{}
	}
	g.result.Variables[n.Name()] = g.expr(n.Definition.Value)
}

func (g *generator) function(f *model.FunctionCallExpression) interface{} {
	if f.Name != "invoke" {
		panic("Can only handle invokes")
	}
	contract.Assert(len(f.Args) > 0)
	name := g.expr(f.Args[0]).(string)
	arguments := map[string]interface{}{}
	if len(f.Args) > 1 {
		args := f.Args[1].(*model.ObjectConsExpression)
		for _, e := range args.Items {
			key := strings.TrimSpace(fmt.Sprintf("%#v", e.Key))
			arguments[key] = g.expr(e.Value)
		}
	} else {
		arguments = nil
	}
	return map[string]struct {
		Function  string                 `yaml:"Function"`
		Arguments map[string]interface{} `yaml:"Arguments,omitempty", json:"Arguments,omitempty"`
	}{"Fn::Invoke": {
		name,
		arguments,
	}}

}
