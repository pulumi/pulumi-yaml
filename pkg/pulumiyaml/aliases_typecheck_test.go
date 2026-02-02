package pulumiyaml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAliasesTypeChecking(t *testing.T) {
	t.Parallel()

	// Test valid string aliases
	t.Run("valid string aliases", func(t *testing.T) {
		text := `
name: test-aliases-valid-string
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - "urn:pulumi:stack::project::test:resource:type::oldName"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "valid string aliases should not produce errors")
	})

	// Test valid object aliases
	t.Run("valid object aliases", func(t *testing.T) {
		text := `
name: test-aliases-valid-object
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - name: oldName
        - type: test:resource:OldType
        - noParent: true
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "valid object aliases should not produce errors")
	})

	// Test mixed string and object aliases
	t.Run("mixed string and object aliases", func(t *testing.T) {
		text := `
name: test-aliases-mixed
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - "urn:pulumi:stack::project::test:resource:type::oldName"
        - name: anotherOldName
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		assert.Empty(t, diags, "mixed string and object aliases should not produce errors")
	})

	// Test invalid field in alias object
	t.Run("invalid field in alias object", func(t *testing.T) {
		text := `
name: test-aliases-invalid-field
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - invalidField: someValue
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "invalid field in alias object should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about invalidField not being a valid property
			var foundInvalidFieldError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "invalidField") {
					foundInvalidFieldError = true
					break
				}
			}
			assert.True(t, foundInvalidFieldError, "should report error about invalid field: %v", diagStrings)
		}
	})

	// Test wrong type for noParent field
	t.Run("wrong type for noParent", func(t *testing.T) {
		text := `
name: test-aliases-wrong-type
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - noParent: "true"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "wrong type for noParent should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about type mismatch
			var foundTypeError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "noParent") || strings.Contains(ds, "bool") || strings.Contains(ds, "string") {
					foundTypeError = true
					break
				}
			}
			assert.True(t, foundTypeError, "should report type error for noParent: %v", diagStrings)
		}
	})

	// Test wrong type for name field
	t.Run("wrong type for name", func(t *testing.T) {
		text := `
name: test-aliases-wrong-name-type
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases:
        - name: 123
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "wrong type for name should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about type mismatch
			var foundTypeError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "name") || strings.Contains(ds, "string") || strings.Contains(ds, "number") {
					foundTypeError = true
					break
				}
			}
			assert.True(t, foundTypeError, "should report type error for name: %v", diagStrings)
		}
	})

	// Test non-array aliases
	t.Run("non-array aliases", func(t *testing.T) {
		text := `
name: test-aliases-not-array
runtime: yaml
resources:
  myResource:
    type: test:resource:type
    properties:
      foo: oof
    options:
      aliases: "urn:pulumi:stack::project::test:resource:type::oldName"
`
		tmpl := yamlTemplate(t, strings.TrimSpace(text))
		diags := testTemplateDiags(t, tmpl, nil)
		if assert.NotEmpty(t, diags, "non-array aliases should produce error") {
			var diagStrings []string
			for _, v := range diags {
				diagStrings = append(diagStrings, diagString(v))
			}
			// Should contain error about expecting array/list
			var foundArrayError bool
			for _, ds := range diagStrings {
				if strings.Contains(ds, "aliases") || strings.Contains(ds, "list") || strings.Contains(ds, "array") {
					foundArrayError = true
					break
				}
			}
			assert.True(t, foundArrayError, "should report error about expecting array: %v", diagStrings)
		}
	})
}
