// Copyright 2022, Pulumi Corporation.  All rights reserved.
//
// Handle configuration types.
package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	yamldiags "github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/diags"
)

type Type interface {
	fmt.Stringer
	// Return the schema equivalent of this type
	Schema() schema.Type
	// Return the pcl equivalent of this type
	Pcl() model.Type

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

func (t typ) Pcl() model.Type {
	switch t.inner {
	case schema.StringType:
		return model.StringType
	case schema.NumberType:
		return model.NumberType
	case schema.BoolType:
		return model.BoolType
	case schema.IntType:
		return model.IntType
	}
	switch t := t.inner.(type) {
	case *schema.ArrayType:
		return model.NewListType(typ{t.ElementType}.Pcl())
	}

	// We should never hit this, but if we do an error should be reported instead of
	// panicking.
	return model.NewOpaqueType("Invalid type :" + t.String())
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

var (
	ErrHeterogeneousList = HeterogeneousListErr{}
	ErrEmptyList         = fmt.Errorf("empty list")
	ErrUnexpectedType    = UnexpectedTypeErr{}
)

type HeterogeneousListErr struct {
	T1 Type
	T2 Type
}

func (e *HeterogeneousListErr) Error() string {
	if e.T1 == nil || e.T2 == nil {
		return "heterogeneous typed lists are not allowed"
	}
	return fmt.Sprintf("heterogeneous typed lists are not allowed: found types %s and %s",
		e.T1, e.T2)
}

func (e *HeterogeneousListErr) Is(err error) bool {
	_, ok := err.(*HeterogeneousListErr)
	return ok
}

type UnexpectedTypeErr struct {
	T interface{}
}

func (e *UnexpectedTypeErr) Error() string {
	if e.T == nil {
		return "unknown type"
	}
	return fmt.Sprintf("unexpected configuration type '%T': valid types are %s",
		e.T, ConfigTypes,
	)
}

func (e *UnexpectedTypeErr) Is(err error) bool {
	_, ok := err.(*UnexpectedTypeErr)
	return ok
}

// Type a go value into a configuration value.
// If an error is returned, it is one of
// - ErrHeterogeneousList
// - ErrEmptyList
// - ErrUnexpectedType
func TypeValue(v interface{}) (Type, error) {
	switch v := v.(type) {
	case string:
		return String, nil
	case float64:
		return Number, nil
	case int:
		return Int, nil
	case bool:
		return Boolean, nil
	case []interface{}:
		var expected Type
		if len(v) == 0 {
			return nil, ErrEmptyList
		}
		switch v[0].(type) {
		case string:
			expected = StringList
		case float64:
			expected = NumberList
		case int:
			expected = IntList
		case bool:
			expected = BooleanList
		}
		for i := 1; i < len(v); i++ {
			if reflect.TypeOf(v[i-1]) != reflect.TypeOf(v[i]) {
				t1, err := TypeValue(v[i-1])
				if err != nil {
					return nil, err
				}
				t2, err := TypeValue(v[i])
				if err != nil {
					return nil, err
				}
				return nil, &HeterogeneousListErr{t1, t2}
			}
		}
		return expected, nil
	case []float64:
		return NumberList, nil
	case []int:
		return IntList, nil
	default:
		return nil, &UnexpectedTypeErr{v}
	}
}
