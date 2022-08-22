// Copyright 2022, Pulumi Corporation.  All rights reserved.
package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		expected Type
	}{
		{"Number", Number},
		{"List<Boolean>", BooleanList},
		{"List< String >", StringList},
		{"List", nil},
		{"List<>", nil},
	}

	for _, c := range cases {
		c := c
		t.Run(c.input, func(t *testing.T) {
			t.Parallel()
			output, ok := Parse(c.input)
			if c.expected == nil {
				assert.False(t, ok)
				assert.Nil(t, output)
			} else {
				assert.True(t, ok)
				assert.Equal(t, c.expected, output)
			}
		})
	}
}

func TestTypeValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    interface{}
		expected Type
		error    error
	}{
		{"foo", String, nil},
		{123, Int, nil},
		{3.14, Number, nil},
		{[]interface{}{123, 345}, IntList, nil},
		{[]interface{}{123, 3.14}, nil, &ErrHeterogeneousList},
		{struct{ s int }{8}, nil, &ErrUnexpectedType},
		{[]int{}, IntList, nil},
		{[]interface{}{}, nil, ErrEmptyList},
		{[]interface{}{false, true}, BooleanList, nil},
	}
	//nolint:paralleltest // false positive that the "c" var isn't used, it is used via "c.input"
	for _, c := range cases {
		c := c
		t.Run(fmt.Sprintf("%v", c.input), func(t *testing.T) {
			t.Parallel()
			typ, err := TypeValue(c.input)
			if c.error == nil {
				assert.NoError(t, err)
				assert.Equal(t, c.expected, typ)
			} else {
				assert.ErrorIs(t, err, c.error)
			}
		})
	}
}
