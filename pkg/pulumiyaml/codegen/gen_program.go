// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/hcl/v2"
	"gopkg.in/yaml.v3"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
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
	g := generator{
		invokeResults: map[*model.FunctionCallExpression]*InvokeCall{},
	}

	g.PrepareForInvokes(program)

	for _, n := range program.Nodes {
		g.genNode(n)
	}

	return g.result, g.diags, nil
}

type generator struct {
	result pulumiyaml.Template
	diags  hcl.Diagnostics
	errs   multierror.Error

	// Because we need a return statement, we might need to call a invoke multiple times.
	invokeResults map[*model.FunctionCallExpression]*InvokeCall
}

type InvokeCall struct {
	name       string
	usedValues codegen.StringSet
}

// Maps names to known invoke usages.
func (g *generator) InvokeNameMap() map[string]*InvokeCall {
	m := make(map[string]*InvokeCall, len(g.invokeResults))
	for _, v := range g.invokeResults {
		m[v.name] = v
	}
	return m
}

// Prepares both the program and the generator for invokes.
// For the program:
//   Invokes are rewritten to handle that a return type must be used.
// For the generator:
//   We memoize what invokes are used so we can correctly give the Return value.
func (g *generator) PrepareForInvokes(program *pcl.Program) {
	for _, n := range program.Nodes {
		switch t := n.(type) {
		case *pcl.LocalVariable:
			if fn, ok := t.Definition.Value.(*model.FunctionCallExpression); ok && fn.Name == pcl.Invoke {
				// An empty list is different then nil
				g.invokeResults[fn] = &InvokeCall{t.Name(), codegen.NewStringSet()}
			}
		case *pcl.ConfigVariable:

		}
		// Pre should collect a list of references to invokes
		calls := g.InvokeNameMap()
		onTraversal := func(f func(*model.ScopeTraversalExpression) *model.ScopeTraversalExpression) func(n model.Expression) (model.Expression, hcl.Diagnostics) {
			return func(n model.Expression) (model.Expression, hcl.Diagnostics) {
				if t, ok := n.(*model.ScopeTraversalExpression); ok {
					return f(t), nil
				}
				return n, nil
			}
		}
		pre := onTraversal(func(t *model.ScopeTraversalExpression) *model.ScopeTraversalExpression {
			if call, ok := calls[t.RootName]; ok {
				path := strings.Split(strings.TrimSpace(fmt.Sprintf("%.v", t)), ".")
				contract.Assertf(len(path) > 1,
					"Invokes must use a field. They cannot just use the result of the invoke")
				// So note the referenced field
				immediateField := path[1]
				contract.Assertf(immediateField != "", "Empty path on %.v", t)
				call.usedValues.Add(path[1])
			}
			return t
		})
		// Post rewrites invoke variable access:
		// If only a single variable was accessed, just use the original name
		// If multiple variables were accessed, create a new named variable for each of them.
		post := onTraversal(func(t *model.ScopeTraversalExpression) *model.ScopeTraversalExpression {
			// This is an invoke
			if call, ok := calls[t.RootName]; ok {
				// Keep the root, but remove the first term of the traversal
				if len(call.usedValues) > 1 {
					panic("Multiple function calls unimplemented")
				}
				t.Parts = t.Parts[1:]
				t.Traversal = t.Traversal[1:]
				// remove the second term, which is handled by the return statement
			}
			return t
		})
		n.VisitExpressions(pre, post)
	}
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
	var dependsOn, ignoreChanges []string
	var protect bool
	if opts := n.Options; opts != nil {
		if opts.Provider != nil {
			provider = g.expr(opts.Provider).(string)
		}
		if opts.Parent != nil {
			parent = g.expr(opts.Parent).(string)
		}
		if opts.DependsOn != nil {
			for _, d := range g.expr(opts.DependsOn).([]interface{}) {
				dependsOn = append(dependsOn, d.(string))
			}
		}
		if opts.IgnoreChanges != nil {
			for _, d := range g.expr(opts.IgnoreChanges).([]interface{}) {
				ignoreChanges = append(ignoreChanges, d.(string))
			}
		}
		if opts.Protect != nil {
			expr := g.expr(opts.Protect)
			b, ok := expr.(bool)
			if s, isString := expr.(string); !ok && isString {
				s = strings.TrimSpace(s)
				if s == "true" {
					b = true
				} else {
					contract.Assertf(s == "false", "'%s' is neither true nor false", s)
				}
			} else {
				contract.Assertf(ok, "Invalid value for '%v': '%[2]v' (%[2]T)", opts.Protect, g.expr(opts.Protect))
			}
			protect = b
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
		DependsOn:               dependsOn,
		IgnoreChanges:           ignoreChanges,
		Import:                  "",
		Parent:                  parent,
		Protect:                 protect,
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
		case model.BoolType:
			r := strings.TrimSpace(fmt.Sprintf("%v", e))
			return r == "true"
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
		return fn("Invoke", g.MustInvoke(f))
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

type Invoke struct {
	Function  string      `yaml:"Function" json:"Function"`
	Arguments interface{} `yaml:"Arguments,omitempty" json:"Arguments,omitempty"`
	Return    string      `yaml:"Return" json:"Return"`
}

func (g *generator) MustInvoke(f *model.FunctionCallExpression) Invoke {
	contract.Assert(f.Name == pcl.Invoke)
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

	// Calculate the return value
	fnInfo := g.invokeResults[f]
	contract.Assertf(len(fnInfo.usedValues) > 0,
		"Invoke assigned to %s has no used values. Dumping known invokes: %#v",
		fnInfo.name, g.invokeResults)
	var retValue string
	if len(fnInfo.usedValues) == 1 {
		retValue = fnInfo.usedValues.SortedValues()[0]
	} else {
		panic("unimplemented")
	}

	return Invoke{
		Function:  name,
		Arguments: arguments,
		Return:    retValue,
	}
}
