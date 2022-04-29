// Copyright 2022, Pulumi Corporation.  All rights reserved.

package syntax

import (
	"github.com/hashicorp/hcl/v2"
)

// A Diagnostic represents a warning or an error to be presented to the user.
type Diagnostic = hcl.Diagnostic

// Warning creates a new warning-level diagnostic from the given subject, summary, and detail.
func Warning(rng *hcl.Range, summary, detail string) *Diagnostic {
	if detail == "" {
		detail = summary
	}
	return &Diagnostic{Severity: hcl.DiagWarning, Subject: rng, Summary: summary, Detail: detail}
}

// Error creates a new error-level diagnostic from the given subject, summary, and detail.
func Error(rng *hcl.Range, summary, detail string) *Diagnostic {
	if detail == "" {
		detail = summary
	}
	return &Diagnostic{Severity: hcl.DiagError, Subject: rng, Summary: summary, Detail: detail}
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
type Diagnostics hcl.Diagnostics

// HasErrors returns true if the list of diagnostics contains any error-level diagnostics.
func (d Diagnostics) HasErrors() bool {
	return hcl.Diagnostics(d).HasErrors()
}

// Error implements the error interface so that Diagnostics values may interoperate with APIs that use errors.
func (d Diagnostics) Error() string {
	return hcl.Diagnostics(d).Error()
}

// Extend appends the given list of diagnostics to the list.
func (d *Diagnostics) Extend(diags ...*Diagnostic) {
	if len(diags) != 0 {
		*d = append(*d, diags...)
	}
}
