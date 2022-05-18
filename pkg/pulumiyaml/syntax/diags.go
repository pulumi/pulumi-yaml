// Copyright 2022, Pulumi Corporation.  All rights reserved.

package syntax

import (
	"fmt"

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
	if detail == "" {
		detail = summary
	}
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
	if detail == "" {
		detail = summary
	}
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
	count := len(d)
	switch {
	case count == 0:
		return "no diagnostics"
	case count == 1:
		return d[0].Error()
	default:
		return fmt.Sprintf("%s, and %d other diagnostic(s)", d[0].Error(), count-1)
	}
}

// Extend appends the given list of diagnostics to the list.
func (d *Diagnostics) Extend(diags ...*Diagnostic) {
	if len(diags) != 0 {
		*d = append(*d, diags...)
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
