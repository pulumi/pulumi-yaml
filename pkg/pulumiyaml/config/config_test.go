package config

import (
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
