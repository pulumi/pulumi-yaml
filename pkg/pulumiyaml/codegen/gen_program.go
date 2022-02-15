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
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
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
	nodes := program.Nodes
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

type yamlLimitationKind string

var (
	Splat    yamlLimitationKind = "splat"
	ToJSON   yamlLimitationKind = "toJSON"
	toBase64 yamlLimitationKind = "toBase64"
)

func (y yamlLimitationKind) Summary() string {
	return fmt.Sprintf("Failed to generate YAML program. Missing %s", string(y))
}

func (g *generator) yamlLimitation(kind yamlLimitationKind) {
	if g.diags == nil {
		g.diags = hcl.Diagnostics{}
	}
	g.diags = g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  kind.Summary(),
	})
}

func (g *generator) missingSchema() {
	g.diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Could not get schema. This might lead to inacurate generation",
	})
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
		if p, ok := g.expr(opts.Provider).(string); ok {
			provider = p
		}
		if p, ok := g.expr(opts.Parent).(string); ok {
			parent = p
		}
	}
	properties := map[string]interface{}{}
	var additionalSecrets []string
	for _, input := range n.Inputs {
		value := input.Value
		if f, ok := value.(*model.FunctionCallExpression); ok && f.Name == "secret" {
			contract.Assert(len(f.Args) == 1)
			additionalSecrets = append(additionalSecrets, input.Name)
			value = f.Args[0]
		}
		properties[input.Name] = g.expr(value)
	}
	if n.Schema == nil {
		g.missingSchema()
		n.Schema = &schema.Resource{}
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
		case model.NoneType:
			return nil
		default:
			r := strings.TrimSpace(fmt.Sprintf("%v", e))
			return r
		}
	case *model.FunctionCallExpression:
		return g.function(e)
	case *model.RelativeTraversalExpression:
		return "${" + strings.TrimSpace(fmt.Sprintf("%.v", e)) + "}"
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
	case *model.TupleConsExpression:
		ls := make([]interface{}, len(e.Expressions))
		for i, e := range e.Expressions {
			ls[i] = g.expr(e)
		}
		return ls
	case *model.ObjectConsExpression:
		obj := map[string]interface{}{}
		for _, e := range e.Items {
			key := strings.TrimSpace(fmt.Sprintf("%#v", e.Key))
			obj[key] = g.expr(e.Value)
		}
		return obj
	case *model.SplatExpression:
		g.yamlLimitation(Splat)
		return nil
	case nil:
		return nil
	default:
		panic(fmt.Sprintf("Unimplimented: %[1]T. Needed for %[1]v", e))
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
	fn := func(name string, body interface{}) map[string]interface{} {
		return map[string]interface{}{
			"Fn::" + name: body,
		}
	}
	switch f.Name {
	case pcl.Invoke:
		contract.Assert(len(f.Args) > 0)
		name := g.expr(f.Args[0]).(string)
		arguments := map[string]interface{}{}
		if len(f.Args) > 1 {
			_, ok := f.Args[1].(*model.ObjectConsExpression)
			contract.Assert(ok)
			arguments = g.expr(f.Args[1]).(map[string]interface{})
		} else {
			arguments = nil
		}
		return fn("Invoke",
			map[string]interface{}{
				"Function":  name,
				"Arguments": arguments,
			})
	case "fileAsset":
		return fn("Asset", map[string]interface{}{
			"File": g.expr(f.Args[0]),
		})
	case "join":
		var args []interface{}
		for _, arg := range f.Args {
			args = append(args, g.expr(arg))
		}
		return fn("Join", args)
	case "toJSON":
		g.yamlLimitation(ToJSON)
		return nil
	case "toBase64":
		g.yamlLimitation(toBase64)
		return nil
	default:
		panic(fmt.Sprintf("function '%s' has not been implemented", f.Name))
	}
}
