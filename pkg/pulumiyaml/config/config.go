// Copyright 2022, Pulumi Corporation.  All rights reserved.
//
// Handle configuration types.
package config

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
)

type Type interface {
	fmt.Stringer
	Schema() schema.Type

	isType()
}

type typ struct{ inner schema.Type }

func (typ) isType() {}

func (t typ) String() string {
	return yamldiags.DisplayType(t.inner)
}

func (t typ) Schema() schema.Type {
	return t.inner
}

var (
	String      Type = typ{schema.StringType}
	StringList       = newList(String)
	Number           = typ{schema.NumberType}
	NumberList       = newList(Number)
	Boolean          = typ{schema.BoolType}
	BooleanList      = newList(Boolean)
	Int              = typ{schema.IntType}
	IntList          = newList(Int)
	Invalid          = typ{&schema.InvalidType{}}
)

type Types []Type

var Primitives = Types{
	String,
	Number,
	Boolean,
}

var ConfigTypes = Types{
	String,
	StringList,
	Number,
	NumberList,
	Boolean,
	BooleanList,
}

func newList(c Type) typ {
	return typ{&schema.ArrayType{ElementType: c.(typ).inner}}
}

func IsValidType(c Type) bool {
	for _, v := range ConfigTypes {
		if v == c {
			return true
		}
	}
	return false
}

func (c Types) String() string {
	l := make([]string, len(c))
	for i, v := range c {
		l[i] = v.String()
	}
	return strings.Join(l, ", ")
}

func Parse(s string) (Type, bool) {
	s = strings.ToLower(s)
	if strings.HasPrefix(s, "list<") && strings.HasSuffix(s, ">") {
		innerString := strings.TrimSuffix(strings.TrimPrefix(s, "list<"), ">")
		inner, ok := Parse(strings.TrimSpace(innerString))
		if !ok {
			return nil, false
		}
		return newList(inner), true
	}

	switch s {
	case "string":
		return String, true
	case "boolean":
		return Boolean, true
	case "number":
		return Number, true
	case "int":
		return Int, true
	default:
		return nil, false
	}
}
