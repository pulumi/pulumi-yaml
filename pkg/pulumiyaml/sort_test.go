// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/syntax"
)

func diagString(d *syntax.Diagnostic) string {
	var s string
	switch {
	case d.Subject != nil:
		s = fmt.Sprintf("%v:%v:%v: %s", d.Subject.Filename, d.Subject.Start.Line, d.Subject.Start.Column, d.Summary)
	case d.Context != nil:
		s = fmt.Sprintf("%v:%v:%v: %s", d.Context.Filename, d.Context.Start.Line, d.Context.End.Line, d.Summary)
	default:
		s = fmt.Sprintf("%v", d.Summary)
	}

	if d.Detail != "" {
		s += fmt.Sprintf("; %v", d.Detail)
	}
	return s
}

func requireNoErrors(t *testing.T, tmpl *ast.TemplateDecl, diags syntax.Diagnostics) {
	if diags.HasErrors() {
		for _, d := range diags {
			if tmpl != nil {
				var buf bytes.Buffer
				w := tmpl.NewDiagnosticWriter(&buf, 0, true)
				err := w.WriteDiagnostic(d.HCL())
				assert.NoError(t, err)
				t.Log(buf.String())
			} else {
				t.Log(diagString(d))
			}
		}
		t.FailNow()
	}
}

func yamlTemplate(t *testing.T, source string) *ast.TemplateDecl {
	pt, diags, err := LoadYAMLBytes("<stdin>", []byte(source))
	require.NoError(t, err)
	requireNoErrors(t, pt, diags)
	return pt
}

func template(t *testing.T, tm *Template) *ast.TemplateDecl {
	pt, diags := LoadTemplate(tm)
	requireNoErrors(t, pt, diags)
	return pt
}

func TestSortOrdered(t *testing.T) {
	t.Parallel()

	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"my-bucket": {
				Type:       "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{},
			},
			"my-object": {
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket":  "${my-bucket.bucketDomainName}",
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
		},
	})
	confNodes := []configNode{}
	resources, diags := topologicallySortedResources(tmpl, confNodes)
	requireNoErrors(t, tmpl, diags)
	names := sortedNames(resources)
	assert.Len(t, names, 2)
	assert.Equal(t, "my-bucket", names[0])
	assert.Equal(t, "my-object", names[1])
}

func TestSortUnordered(t *testing.T) {
	t.Parallel()

	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"my-object": {
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket":  "${my-bucket.bucketDomainName}",
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
			"my-bucket": {
				Type:       "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{},
			},
		},
	})
	confNodes := []configNode{}
	resources, diags := topologicallySortedResources(tmpl, confNodes)
	requireNoErrors(t, tmpl, diags)
	names := sortedNames(resources)
	assert.Len(t, names, 2)
	assert.Equal(t, "my-bucket", names[0])
	assert.Equal(t, "my-object", names[1])
}

func TestSortErrorCycle(t *testing.T) {
	t.Parallel()

	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"my-object": {
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket":  "${my-bucket.bucketDomainName}",
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
			"my-bucket": {
				Type: "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{
					"Invalid": "${my-object.id}",
				},
			},
		},
	})
	confNodes := []configNode{}
	_, err := topologicallySortedResources(tmpl, confNodes)
	assert.Error(t, err)
}

func sortedNames(rs []graphNode) []string {
	names := make([]string, len(rs))
	for i, kvp := range rs {
		names[i] = kvp.key().Value
	}
	return names
}
