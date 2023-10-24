// Copyright 2022, Pulumi Corporation.  All rights reserved.

package diags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEditDistance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b     string
		expected int
	}{
		{"vpcId", "cpcId", 1},
		{"vpcId", "foo", 5},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, editDistance(c.a, c.b))
	}
}

func TestSortByEditDistance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		words      []string
		comparedTo string
		expected   []string
	}{
		{[]string{}, "test", []string{}},
		{[]string{"", "", ""}, "test", []string{"", "", ""}},
		{[]string{"test", "test2"}, "test", []string{"test", "test2"}},
		{[]string{"test2", "test"}, "test", []string{"test", "test2"}},
		{[]string{"test2", "test", "test2"}, "test", []string{"test", "test2", "test2"}},
		{[]string{"c", "b", "a"}, "test", []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		assert.Equalf(t, c.expected, sortByEditDistance(c.words, c.comparedTo), "sortByEditDistance(%v, %v)", c.words, c.comparedTo)
	}
}

func TestDisplayList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		h         []string
		conjuctor string
		expected  string
	}{
		{[]string{}, "and", ""},
		{[]string{"a"}, "and", "a"},
		{[]string{"a", "b"}, "and", "a and b"},
		{[]string{"a", "b"}, "or", "a or b"},
		{[]string{"a", "b"}, "random", "a random b"},
		{[]string{"a", "b", "c"}, "and", "a, b and c"},
	}
	for _, c := range cases {
		assert.Equalf(t, c.expected, displayList(c.h, c.conjuctor), "displayList(%v, %v)", c.h, c.conjuctor)
	}
}
