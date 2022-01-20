package pulumiyaml

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-yaml/pulumiyaml/ast"
	"github.com/pulumi/pulumi-yaml/pulumiyaml/syntax"
)

func diagString(d *hcl.Diagnostic) string {
	return fmt.Sprintf("%v:%v:%v: %s", d.Subject.Filename, d.Subject.Start.Line, d.Subject.Start.Column, d.Summary)
}

func requireNoErrors(t *testing.T, diags syntax.Diagnostics) {
	if !assert.False(t, diags.HasErrors()) {
		for _, d := range diags {
			t.Log(diagString(d))
		}
		t.FailNow()
	}
}

func yamlTemplate(t *testing.T, source string) *ast.TemplateDecl {
	pt, diags, err := LoadYAMLBytes("<stdin>", []byte(source))
	require.NoError(t, err)
	requireNoErrors(t, diags)
	return pt
}

func template(t *testing.T, tm *Template) *ast.TemplateDecl {
	pt, diags := LoadTemplate(tm)
	requireNoErrors(t, diags)
	return pt
}

func TestSortOrdered(t *testing.T) {
	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"my-bucket": {
				Type:       "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{},
			},
			"my-object": {
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-bucket", "bucketDomainName"},
					},
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
		},
	})
	resources, diags := topologicallySortedResources(tmpl)
	requireNoErrors(t, diags)
	names := sortedNames(resources)
	assert.Len(t, names, 2)
	assert.Equal(t, "my-bucket", names[0])
	assert.Equal(t, "my-object", names[1])
}

func TestSortUnordered(t *testing.T) {
	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"my-object": {
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-bucket", "bucketDomainName"},
					},
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
	resources, diags := topologicallySortedResources(tmpl)
	requireNoErrors(t, diags)
	names := sortedNames(resources)
	assert.Len(t, names, 2)
	assert.Equal(t, "my-bucket", names[0])
	assert.Equal(t, "my-object", names[1])
}

func TestSortErrorCycle(t *testing.T) {
	tmpl := template(t, &Template{
		Resources: map[string]*Resource{
			"my-object": {
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-bucket", "bucketDomainName"},
					},
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
			"my-bucket": {
				Type: "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{
					"Invalid": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-object", "id"},
					},
				},
			},
		},
	})
	_, err := topologicallySortedResources(tmpl)
	assert.Error(t, err)
}

func sortedNames(rs []ast.ResourcesMapEntry) []string {
	names := make([]string, len(rs))
	for i, kvp := range rs {
		names[i] = kvp.Key.Value
	}
	return names
}
