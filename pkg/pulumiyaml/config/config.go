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
	StringList       = typ{&schema.ArrayType{ElementType: schema.StringType}}
	Number           = typ{schema.NumberType}
	NumberList       = typ{&schema.ArrayType{ElementType: schema.NumberType}}
	Boolean          = typ{schema.BoolType}
	BooleanList      = typ{&schema.ArrayType{ElementType: schema.NumberType}}
	Int              = typ{schema.IntType}
	IntList          = typ{&schema.ArrayType{ElementType: schema.IntType}}
	Invalid          = typ{&schema.InvalidType{}}
)

type Types []Type

var Primitives = Types{
	String,
	Number,
	Int,
	Boolean,
}

var ConfigTypes = Types{
	String,
	StringList,
	Number,
	NumberList,
	Int,
	IntList,
	Boolean,
	BooleanList,
}

func newList(c Type) typ {
	// This is necessary to preserve switch equality
	switch c {
	case String:
		return StringList
	case Number:
		return NumberList
	case Int:
		return IntList
	case Boolean:
		return BooleanList
	default:
		return typ{&schema.ArrayType{ElementType: c.(typ).inner}}
	}
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
