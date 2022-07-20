// Copyright 2022, Pulumi Corporation.  All rights reserved.

package ast

import (
	"fmt"
	"io"
	"reflect"
	"unicode"

	"github.com/hashicorp/hcl/v2"

	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type declNode struct {
	syntax syntax.Node
}

func decl(node syntax.Node) declNode {
	return declNode{node}
}

func (x *declNode) Syntax() syntax.Node {
	if x == nil {
		return nil
	}
	return x.syntax
}

type parseDecl interface {
	parse(name string, node syntax.Node) syntax.Diagnostics
}

type recordDecl interface {
	recordSyntax() *syntax.Node
}

type StringListDecl struct {
	declNode

	Elements []*StringExpr
}

type nonNilDecl interface {
	defaultValue() interface{}
}

func (d *StringListDecl) GetElements() []*StringExpr {
	if d == nil {
		return nil
	}
	return d.Elements
}

func (d *StringListDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	list, ok := node.(*syntax.ListNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be a list", name), "")}
	}

	var diags syntax.Diagnostics

	elements := make([]*StringExpr, list.Len(), list.Len())
	for i := range elements {
		ename := fmt.Sprintf("%s[%d]", name, i)
		ediags := parseField(ename, reflect.ValueOf(&elements[i]).Elem(), list.Index(i))
		diags.Extend(ediags...)
	}
	d.Elements = elements

	return diags
}

type ConfigMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  *ConfigParamDecl
}

type ConfigMapDecl struct {
	declNode

	Entries []ConfigMapEntry
}

func (d *ConfigMapDecl) defaultValue() interface{} {
	return &ConfigMapDecl{}
}

func (d *ConfigMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]ConfigMapEntry, obj.Len(), obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		var v *ConfigParamDecl
		vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
		vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = ConfigMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type VariablesMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  Expr
}

type VariablesMapDecl struct {
	declNode

	Entries []VariablesMapEntry
}

func (d *VariablesMapDecl) defaultValue() interface{} {
	return &VariablesMapDecl{}
}

func (d *VariablesMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]VariablesMapEntry, obj.Len(), obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		v, vdiags := ParseExpr(kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = VariablesMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type ResourcesMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  *ResourceDecl
}

type ResourcesMapDecl struct {
	declNode

	Entries []ResourcesMapEntry
}

func (d *ResourcesMapDecl) defaultValue() interface{} {
	return &ResourcesMapDecl{}
}

func (d *ResourcesMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]ResourcesMapEntry, obj.Len(), obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		var v *ResourceDecl
		vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
		vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = ResourcesMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type PropertyMapEntry struct {
	syntax syntax.ObjectPropertyDef
	Key    *StringExpr
	Value  Expr
}

type PropertyMapDecl struct {
	declNode

	Entries []PropertyMapEntry
}

func (d *PropertyMapDecl) defaultValue() interface{} {
	return &PropertyMapDecl{}
}

func (d *PropertyMapDecl) parse(name string, node syntax.Node) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", name), "")}
	}

	var diags syntax.Diagnostics

	entries := make([]PropertyMapEntry, obj.Len(), obj.Len())
	for i := range entries {
		kvp := obj.Index(i)

		var v Expr
		vname := fmt.Sprintf("%s.%s", name, kvp.Key.Value())
		vdiags := parseField(vname, reflect.ValueOf(&v).Elem(), kvp.Value)
		diags.Extend(vdiags...)

		entries[i] = PropertyMapEntry{
			syntax: kvp,
			Key:    StringSyntax(kvp.Key),
			Value:  v,
		}
	}
	d.Entries = entries

	return diags
}

type ConfigParamDecl struct {
	declNode

	Type    *StringExpr
	Secret  *BooleanExpr
	Default Expr
}

func (d *ConfigParamDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func ConfigParamSyntax(node *syntax.ObjectNode, typ *StringExpr,
	secret *BooleanExpr, defaultValue Expr) *ConfigParamDecl {

	return &ConfigParamDecl{
		declNode: decl(node),
		Type:     typ,
		Secret:   secret,
		Default:  defaultValue,
	}
}

func ConfigParam(typ *StringExpr, defaultValue Expr, secret *BooleanExpr) *ConfigParamDecl {
	return ConfigParamSyntax(nil, typ, secret, defaultValue)
}

type ResourceOptionsDecl struct {
	declNode

	AdditionalSecretOutputs *StringListDecl
	Aliases                 *StringListDecl
	CustomTimeouts          *CustomTimeoutsDecl
	DeleteBeforeReplace     *BooleanExpr
	DependsOn               Expr
	IgnoreChanges           *StringListDecl
	Import                  *StringExpr
	Parent                  Expr
	Protect                 *BooleanExpr
	Provider                Expr
	Providers               Expr
	Version                 *StringExpr
	PluginDownloadURL       *StringExpr
	ReplaceOnChanges        *StringListDecl
	RetainOnDelete          *BooleanExpr
}

func (d *ResourceOptionsDecl) defaultValue() interface{} {
	return &ResourceOptionsDecl{}
}

func (d *ResourceOptionsDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func ResourceOptionsSyntax(node *syntax.ObjectNode,
	additionalSecretOutputs, aliases *StringListDecl, customTimeouts *CustomTimeoutsDecl,
	deleteBeforeReplace *BooleanExpr, dependsOn Expr, ignoreChanges *StringListDecl, importID *StringExpr,
	parent Expr, protect *BooleanExpr, provider, providers Expr, version *StringExpr,
	pluginDownloadURL *StringExpr, replaceOnChanges *StringListDecl,
	retainOnDelete *BooleanExpr) ResourceOptionsDecl {

	return ResourceOptionsDecl{
		declNode:                decl(node),
		AdditionalSecretOutputs: additionalSecretOutputs,
		Aliases:                 aliases,
		CustomTimeouts:          customTimeouts,
		DeleteBeforeReplace:     deleteBeforeReplace,
		DependsOn:               dependsOn,
		IgnoreChanges:           ignoreChanges,
		Import:                  importID,
		Parent:                  parent,
		Protect:                 protect,
		Provider:                provider,
		Version:                 version,
		PluginDownloadURL:       pluginDownloadURL,
		ReplaceOnChanges:        replaceOnChanges,
		RetainOnDelete:          retainOnDelete,
	}
}

func ResourceOptions(additionalSecretOutputs, aliases *StringListDecl,
	customTimeouts *CustomTimeoutsDecl, deleteBeforeReplace *BooleanExpr,
	dependsOn Expr, ignoreChanges *StringListDecl, importID *StringExpr, parent Expr,
	protect *BooleanExpr, provider, providers Expr, version *StringExpr, pluginDownloadURL *StringExpr,
	replaceOnChanges *StringListDecl, retainOnDelete *BooleanExpr) ResourceOptionsDecl {

	return ResourceOptionsSyntax(nil, additionalSecretOutputs, aliases, customTimeouts,
		deleteBeforeReplace, dependsOn, ignoreChanges, importID, parent, protect, provider, providers,
		version, pluginDownloadURL, replaceOnChanges, retainOnDelete)
}

type GetResourceDecl struct {
	declNode
	// We need to call the field Id instead of ID because we want the derived user field to be id instead of iD
	Id    Expr //nolint:revive
	State PropertyMapDecl
}

func (d *GetResourceDecl) defaultValue() interface{} {
	return &GetResourceDecl{}
}

func (d *GetResourceDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func GetResourceSyntax(node *syntax.ObjectNode, id *StringExpr, state PropertyMapDecl) GetResourceDecl {
	return GetResourceDecl{
		declNode: decl(node),
		Id:       id,
		State:    state,
	}
}

func GetResource(id *StringExpr, state PropertyMapDecl) GetResourceDecl {
	return GetResourceSyntax(nil, id, state)
}

type ResourceDecl struct {
	declNode

	Type       *StringExpr
	Properties PropertyMapDecl
	Options    ResourceOptionsDecl
	Get        GetResourceDecl
}

func (d *ResourceDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

// The names of exported fields.
func (*ResourceDecl) Fields() []string {
	return []string{"type", "properties", "options", "get"}
}

func ResourceSyntax(node *syntax.ObjectNode, typ *StringExpr,
	properties PropertyMapDecl, options ResourceOptionsDecl, get GetResourceDecl) *ResourceDecl {
	return &ResourceDecl{
		declNode:   decl(node),
		Type:       typ,
		Properties: properties,
		Options:    options,
		Get:        get,
	}
}

func Resource(typ *StringExpr, properties PropertyMapDecl, options ResourceOptionsDecl, get GetResourceDecl) *ResourceDecl {
	return ResourceSyntax(nil, typ, properties, options, get)
}

type CustomTimeoutsDecl struct {
	declNode

	Create *StringExpr
	Update *StringExpr
	Delete *StringExpr
}

func (d *CustomTimeoutsDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

func CustomTimeoutsSyntax(node *syntax.ObjectNode, create, update, delete *StringExpr) *CustomTimeoutsDecl {
	return &CustomTimeoutsDecl{
		declNode: declNode{syntax: node},
		Create:   create,
		Update:   update,
		Delete:   delete,
	}
}

func CustomTimeouts(create, update, delete *StringExpr) *CustomTimeoutsDecl {
	return CustomTimeoutsSyntax(nil, create, update, delete)
}

// A TemplateDecl represents a Pulumi YAML template.
type TemplateDecl struct {
	source []byte

	syntax syntax.Node

	Description   *StringExpr
	Configuration ConfigMapDecl
	Variables     VariablesMapDecl
	Resources     ResourcesMapDecl
	Outputs       PropertyMapDecl
}

func (d *TemplateDecl) Syntax() syntax.Node {
	if d == nil {
		return nil
	}
	return d.syntax
}

func (d *TemplateDecl) recordSyntax() *syntax.Node {
	return &d.syntax
}

// NewDiagnosticWriter returns a new hcl.DiagnosticWriter that can be used to print diagnostics associated with the
// template.
func (d *TemplateDecl) NewDiagnosticWriter(w io.Writer, width uint, color bool) hcl.DiagnosticWriter {
	fileMap := map[string]*hcl.File{}
	if d.source != nil {
		if s := d.syntax; s != nil {
			fileMap[s.Syntax().Range().Filename] = &hcl.File{Bytes: d.source}
		}
	}
	return newDiagnosticWriter(w, fileMap, width, color)
}

func TemplateSyntax(node *syntax.ObjectNode, description *StringExpr, configuration ConfigMapDecl,
	variables VariablesMapDecl, resources ResourcesMapDecl, outputs PropertyMapDecl) *TemplateDecl {

	return &TemplateDecl{
		syntax:        node,
		Description:   description,
		Configuration: configuration,
		Variables:     variables,
		Resources:     resources,
		Outputs:       outputs,
	}
}

func Template(description *StringExpr, configuration ConfigMapDecl, variables VariablesMapDecl, resources ResourcesMapDecl,
	outputs PropertyMapDecl) *TemplateDecl {

	return TemplateSyntax(nil, description, configuration, variables, resources, outputs)
}

// ParseTemplate parses a template from the given syntax node. The source text is optional, and is only used to print
// diagnostics.
func ParseTemplate(source []byte, node syntax.Node) (*TemplateDecl, syntax.Diagnostics) {
	template := TemplateDecl{source: source}

	diags := parseRecord("template", &template, node, false)
	return &template, diags
}

var parseDeclType = reflect.TypeOf((*parseDecl)(nil)).Elem()
var nonNilDeclType = reflect.TypeOf((*nonNilDecl)(nil)).Elem()
var recordDeclType = reflect.TypeOf((*recordDecl)(nil)).Elem()
var exprType = reflect.TypeOf((*Expr)(nil)).Elem()

func parseField(name string, dest reflect.Value, node syntax.Node) syntax.Diagnostics {
	if node == nil {
		return nil
	}

	var v reflect.Value
	var diags syntax.Diagnostics

	if dest.CanAddr() && dest.Addr().Type().AssignableTo(nonNilDeclType) {
		// destination is T, and must be a record type (right now)
		defaultValue := (dest.Addr().Interface().(nonNilDecl)).defaultValue()
		switch x := defaultValue.(type) {
		case parseDecl:
			pdiags := x.parse(name, node)
			diags.Extend(pdiags...)
			v = reflect.ValueOf(defaultValue).Elem().Convert(dest.Type())
		case recordDecl:
			pdiags := parseRecord(name, x, node, true)
			diags.Extend(pdiags...)
			v = reflect.ValueOf(defaultValue).Elem().Convert(dest.Type())
		}
		dest.Set(v)
		return diags
	}

	switch {
	case dest.Type().AssignableTo(parseDeclType):
		// assume that dest is *T
		v = reflect.New(dest.Type().Elem())
		pdiags := v.Interface().(parseDecl).parse(name, node)
		diags.Extend(pdiags...)
	case dest.Type().AssignableTo(recordDeclType):
		// assume that dest is *T
		v = reflect.New(dest.Type().Elem())
		rdiags := parseRecord(name, v.Interface().(recordDecl), node, true)
		diags.Extend(rdiags...)
	case dest.Type().AssignableTo(exprType):
		x, xdiags := ParseExpr(node)
		diags.Extend(xdiags...)
		if diags.HasErrors() {
			return diags
		}

		xv := reflect.ValueOf(x)
		if !xv.Type().AssignableTo(dest.Type()) {
			diags.Extend(exprFieldTypeMismatchError(name, dest.Interface(), x))
		} else {
			v = xv
		}
	default:
		panic(fmt.Errorf("unexpected field of type %T", dest.Interface()))
	}

	if !diags.HasErrors() {
		dest.Set(v)
	}
	return diags
}

func parseRecord(objName string, dest recordDecl, node syntax.Node, noMatchWarning bool) syntax.Diagnostics {
	obj, ok := node.(*syntax.ObjectNode)
	if !ok {
		return syntax.Diagnostics{syntax.NodeError(node, fmt.Sprintf("%v must be an object", objName), "")}
	}
	*dest.recordSyntax() = obj
	contract.Assertf(*dest.recordSyntax() == obj, "%s.recordSyntax took by value, so the assignment failed", objName)

	v := reflect.ValueOf(dest).Elem()
	t := v.Type()

	var diags syntax.Diagnostics
	for i := 0; i < obj.Len(); i++ {
		kvp := obj.Index(i)

		key := kvp.Key.Value()
		var hasMatch bool
		for _, name := range []string{key, title(key)} {
			if f, ok := t.FieldByName(name); ok && f.IsExported() {
				fdiags := parseField(key, v.FieldByIndex(f.Index), kvp.Value)
				diags.Extend(fdiags...)
				hasMatch = true
				break
			}
		}
		if !hasMatch && noMatchWarning {
			var fieldNames []string
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.IsExported() {
					name := []rune(f.Name)
					name[0] = unicode.ToLower(name[0])
					fieldNames = append(fieldNames, fmt.Sprintf("'%s'", string(name)))
				}
			}
			formatter := yamldiags.NonExistantFieldFormatter{
				ParentLabel: fmt.Sprintf("Object '%s'", objName),
				Fields:      fieldNames,
			}
			msg, detail := formatter.MessageWithDetail(key, fmt.Sprintf("Field '%s'", key))
			nodeError := syntax.NodeError(kvp.Key, msg, detail)
			nodeError.Severity = hcl.DiagWarning
			diags = append(diags, nodeError)
		}

	}

	return diags
}

func exprFieldTypeMismatchError(name string, expected interface{}, actual Expr) *syntax.Diagnostic {
	var typeName string
	switch expected.(type) {
	case *NullExpr:
		typeName = "null"
	case *BooleanExpr:
		typeName = "a boolean value"
	case *NumberExpr:
		typeName = "a number"
	case *StringExpr:
		typeName = "a string"
	case *SymbolExpr:
		typeName = "a symbol"
	case *InterpolateExpr:
		typeName = "an interpolated string"
	case *ListExpr:
		typeName = "a list"
	case *ObjectExpr:
		typeName = "an object"
	case BuiltinExpr:
		typeName = "a builtin function call"
	default:
		typeName = fmt.Sprintf("a %T", expected)
	}
	return ExprError(actual, fmt.Sprintf("%v must be %v", name, typeName), "")
}

func title(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(append([]rune{unicode.ToUpper(runes[0])}, runes[1:]...))
}
