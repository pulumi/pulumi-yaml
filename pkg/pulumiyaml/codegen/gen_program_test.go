// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

func TestGenerateProgram(t *testing.T) {

	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{}
		for _, tt := range tests {
			switch tt.Directory {
			case "azure-sa":
				// Reason: has dependencies between config variables
			case "aws-eks", "aws-s3-folder":
				// Reason: missing splat
				//
				// Note: aws-s3-folder errors with
				// 14,27-52: the asset parameter must be a string literal; the asset parameter must be a string literal
				// But the actual error is that it is using a Splat operator.
			default:
				l = append(l, tt)
			}
		}
		return l
	}

	check := func(t *testing.T, output string, _ codegen.StringSet) {
		file, err := os.ReadFile(output)
		assert.NoError(t, err)
		templateDecl, diags, err := pulumiyaml.LoadYAMLBytes(output, file)
		assert.NoError(t, err)
		assert.Falsef(t, diags.HasErrors(), "%s", diags.Error())
		err = pulumi.RunErr(func(ctx *pulumi.Context) error {
			return pulumiyaml.RunTemplate(ctx, templateDecl)
		}, pulumi.WithMocks("test", "gen", &testMonitor{}))
		assert.NoError(t, err)
	}

	c := struct {
		StorageAccountNameParam string `json:"project:storageAccountNameParam"`
		ResourceGroupNameParam  string `json:"project:resourceGroupNameParam"`
	}{
		"storageAccountNameParam",
		"resourceGroupNameParam",
	}
	config, err := json.Marshal(c)
	assert.NoError(t, err, "Failed to marshal fake config")
	t.Setenv("PULUMI_CONFIG", string(config))
	fmt.Printf("config: '%s'\n", string(config))

	test.TestProgramCodegen(t,
		test.ProgramCodegenOptions{
			Language:   "yaml",
			Extension:  "yaml",
			OutputFile: "Main.yaml",
			Check:      check,
			GenProgram: GenerateProgram,
			TestCases:  filter(test.PulumiPulumiProgramTests),
		},
	)
}

type testMonitor struct{}

func (m *testMonitor) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	switch args.Token {
	case "aws:index/getAmi:getAmi":
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"id": "1234",
		}), nil

	// For azure-sa-pp
	case "azure:core/getResourceGroup:getResourceGroup":
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"location": "just-a-location",
		}), nil

	// For output-funcs-aws
	case "aws:ec2/getPrefixList:getPrefixList":
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"cidrBlocks": []string{"some-list"},
		}), nil

	// For aws-fargate
	case "aws:ec2/getSubnetIds:getSubnetIds":
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"ids": []string{"some-ids"},
		}), nil
	case "aws:ec2/getVpc:getVpc":
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"id": "some-id",
		}), nil

	}
	return resource.PropertyMap{}, nil
}

func (m *testMonitor) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	switch args.Name {
	case "bucket":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"loggings": []interface{}{
				map[string]string{
					"targetBucket": "foo",
				},
			},
		}), nil
	case "logs":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"bucket": "foo",
		}), nil
	case "server":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"publicIp":  "some-public-ip",
			"publicDns": "some-public-dns",
		}), nil
	case "securityGroup":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"name": "some-name",
		}), nil

	// For azure-sa-pp
	case "storageAccountResource":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"name": "some-name",
		}), nil

	// For aws-fargate
	case "webSecurityGroup":
		return args.Name, resource.NewPropertyMapFromMap(map[string]interface{}{
			"id": "some-id",
		}), nil

	}

	return args.Name, resource.PropertyMap{}, nil
}
