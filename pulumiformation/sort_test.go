package pulumiformation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortOrdered(t *testing.T) {
	tmpl := &Template{
		Resources: map[string]*Resource{
			"my-bucket": &Resource{
				Type:       "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{},
			},
			"my-object": &Resource{
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
	}
	resources, err := topologicallySortedResources(tmpl)
	assert.NoError(t, err)
	assert.Len(t, resources, 2)
	assert.Equal(t, "my-bucket", resources[0])
	assert.Equal(t, "my-object", resources[1])
}

func TestSortUnordered(t *testing.T) {
	tmpl := &Template{
		Resources: map[string]*Resource{
			"my-object": &Resource{
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-bucket", "bucketDomainName"},
					},
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
			"my-bucket": &Resource{
				Type:       "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{},
			},
		},
	}
	resources, err := topologicallySortedResources(tmpl)
	assert.NoError(t, err)
	assert.Len(t, resources, 2)
	assert.Equal(t, "my-bucket", resources[0])
	assert.Equal(t, "my-object", resources[1])
}

func TestSortErrorCycle(t *testing.T) {
	tmpl := &Template{
		Resources: map[string]*Resource{
			"my-object": &Resource{
				Type: "aws:s3/bucketObject:BucketObject",
				Properties: map[string]interface{}{
					"Bucket": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-bucket", "bucketDomainName"},
					},
					"Content": "Hello, world!",
					"Key":     "info.txt",
				},
			},
			"my-bucket": &Resource{
				Type: "aws:s3/bucket:Bucket",
				Properties: map[string]interface{}{
					"Invalid": map[string]interface{}{
						"Fn::GetAtt": []interface{}{"my-object", "id"},
					},
				},
			},
		},
	}
	_, err := topologicallySortedResources(tmpl)
	assert.Error(t, err)
}
