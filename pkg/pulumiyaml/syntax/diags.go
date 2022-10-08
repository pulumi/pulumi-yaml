// Copyright 2022, Pulumi Corporation.  All rights reserved.

package syntax

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
)

// A Diagnostic represents a warning or an error to be presented to the user.
type Diagnostic struct {
	hcl.Diagnostic

	// Whether the diagnostic has been shown to the user
	Shown bool
}

// WithContext adds context without mutating the receiver.
func (d Diagnostic) WithContext(rng *hcl.Range) *Diagnostic {
	d.Context = rng
	return &d
}

func (d Diagnostic) HCL() *hcl.Diagnostic {
	return &d.Diagnostic
}

// Warning creates a new warning-level diagnostic from the given subject, summary, and detail.
func Warning(rng *hcl.Range, summary, detail string) *Diagnostic {
	return &Diagnostic{
		Diagnostic: hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Subject:  rng,
			Summary:  summary,
			Detail:   detail,
		},
	}
}

// Error creates a new error-level diagnostic from the given subject, summary, and detail.
func Error(rng *hcl.Range, summary, detail string) *Diagnostic {
	return &Diagnostic{
		Diagnostic: hcl.Diagnostic{Severity: hcl.DiagError, Subject: rng, Summary: summary, Detail: detail},
	}
}

// NodeError creates a new error-level diagnostic from the given node, summary, and detail. If the node is non-nil,
// the diagnostic will be associated with the range of its associated syntax, if any.
func NodeError(node Node, summary, detail string) *Diagnostic {
	var rng *hcl.Range
	if node != nil {
		rng = node.Syntax().Range()
	}
	return Error(rng, summary, detail)
}

// Diagnostics is a list of diagnostics.
type Diagnostics []*Diagnostic

// HasErrors returns true if the list of diagnostics contains any error-level diagnostics.
func (d Diagnostics) HasErrors() bool {
	for _, diag := range d {
		if diag.Severity == hcl.DiagError {
			return true
		}
	}
	return false
}

// Error implements the error interface so that Diagnostics values may interoperate with APIs that use errors.
func (d Diagnostics) Error() string {
	switch len(d) {
	case 0:
		return "no diagnostics"
	case 1:
		return d[0].Error()
	default:
		sort.Slice(d, func(i, j int) bool {
			return d[i].Severity < d[j].Severity
		})
		var sb strings.Builder
		for _, diag := range d {
			if diag.Severity == hcl.DiagError {
				sb.WriteString("\n-error: ")
			} else {
				sb.WriteString("\n-warning: ")
			}
			sb.WriteString(fmt.Sprintf("%s", diag.Error()))
		}
		return sb.String()
	}
}

// Extend appends the given list of diagnostics to the list.
func (d *Diagnostics) Extend(diags ...*Diagnostic) {
	if len(diags) != 0 {
		for _, diag := range diags {
			if diag != nil {
				*d = append(*d, diag)
			}
		}
	}
}

func (d *Diagnostics) HCL() hcl.Diagnostics {
	if d == nil {
		return nil
	}
	a := make(hcl.Diagnostics, 0, len(*d))
	for _, diag := range *d {
		a = append(a, diag.HCL())
	}
	return a
}

func (d Diagnostics) Unshown() *Diagnostics {
	diags := Diagnostics{}
	for _, diag := range d {
		if !diag.Shown {
			diags = append(diags, diag)
		}
	}
	return &diags
}

// Inform the user that they did not conform with the expected capitalization style. If
// `expected` matches `found`, then `nil` is returned. This allows
// `Diagnostics.Extend(UnexpectedCasing(location, expected, found))` without checking if
// expected equals found.
func UnexpectedCasing(rng *hcl.Range, expected, found string) *Diagnostic {
	if expected == found {
		return nil
	}
	summary := fmt.Sprintf("'%s' looks like a miscapitalization of '%s'", found, expected)
	detail := "A future version of Pulumi YAML will enforce camelCase fields. See https://github.com/pulumi/pulumi-yaml/issues/355 for details."
	return Warning(rng, summary, detail)
}
