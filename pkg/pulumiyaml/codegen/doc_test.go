// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
)

func TestActuallDocLanguageHelper(t *testing.T) {
	t.Parallel()
	func(codegen.DocLanguageHelper) {}(DocLanguageHelper{})
}
