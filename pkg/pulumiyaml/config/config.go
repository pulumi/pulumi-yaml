// Copyright 2022, Pulumi Corporation.  All rights reserved.
//
// Handle configuration types.
package config

import (
	"fmt"
	"strings"
)

type Type interface {
	fmt.Stringer
}

type Primitive string

var (
	String      Primitive = "String"
	StringList            = newList(String)
	Number      Primitive = "Number"
	NumberList            = newList(Number)
	Boolean     Primitive = "Boolean"
	BooleanList           = newList(Boolean)

	Invalid Primitive = "Invalid"
)

func (p Primitive) String() string {
	return string(p)
}

type Types []Type

type List struct {
	element Type
}

func (l List) Element() Type {
	return l.element
}

func (l List) String() string {
	return fmt.Sprintf("List<%s>", l.element)
}

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

func newList(c Type) List {
	return List{element: c}
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
	if strings.HasPrefix(strings.ToLower(s), "list<") && strings.HasSuffix(s, ">") {
		innerString := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(s, "List<"), "list<"), ">")
		inner, ok := Parse(strings.TrimSpace(innerString))
		if !ok {
			return nil, false
		}
		return newList(inner), true
	}

	switch s {
	case "String", "string":
		return String, true
	case "Boolean", "boolean":
		return Boolean, true
	case "Number", "number":
		return Number, true
	default:
		return nil, false
	}
}
