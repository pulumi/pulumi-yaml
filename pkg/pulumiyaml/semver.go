// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"github.com/blang/semver"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
)

func ParseVersion(expr ast.Expr) (*semver.Version, error) {
	switch v := expr.(type) {
	case *ast.StringExpr:
		if v == nil || v.Value == "" {
			return nil, nil
		}

		version, err := semver.ParseTolerant(v.Value)
		if err != nil {
			return nil, err
		}

		return &version, nil
	}

	return nil, nil
}
