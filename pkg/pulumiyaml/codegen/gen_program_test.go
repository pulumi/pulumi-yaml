// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/ast"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/pkg/v3/codegen/dotnet"
	gogen "github.com/pulumi/pulumi/pkg/v3/codegen/go"
	"github.com/pulumi/pulumi/pkg/v3/codegen/nodejs"
	"github.com/pulumi/pulumi/pkg/v3/codegen/python"
)

type ConvertFunc = func(t *testing.T, template *ast.TemplateDecl, dir string)

func convertTo(name string, generator GenerateFunc) ConvertFunc {
	return func(t *testing.T, template *ast.TemplateDecl, dir string) {
		t.Run(name, func(t *testing.T) {
			files, diags, err := ConvertTemplate(template, generator)
			require.NoError(t, err, "Failed to convert")
			assert.False(t, diags.HasErrors(), diags.Error())
			for path, bytes := range files {
				path = filepath.Join(dir, name, filepath.FromSlash(path))
				err = os.MkdirAll(filepath.Dir(path), 0700)
				require.NoError(t, err)
				err = os.WriteFile(path, bytes, 0600)
				require.NoError(t, err)
			}
		})
	}
}

var convertNodeJS = convertTo("nodejs", nodejs.GenerateProgram)
var convertPython = convertTo("python", python.GenerateProgram)
var convertGolang = convertTo("go", gogen.GenerateProgram)
var convertDotnet = convertTo("dotnet", dotnet.GenerateProgram)

func TestGenerateExamples(t *testing.T) {
	examplesPath := filepath.Join("..", "..", "..", "examples")
	examples, err := ioutil.ReadDir(examplesPath)
	require.NoError(t, err)
	for _, dir := range examples {
		t.Run(dir.Name(), func(t *testing.T) {
			main := filepath.Join(examplesPath, dir.Name(), "Pulumi.yaml")
			template, diags, err := pulumiyaml.LoadFile(main)
			require.NoError(t, err, "Loading file: %s", main)
			assert.False(t, diags.HasErrors(), diags.Error())
			outDir := filepath.Join("..", "testing", "test", "testdata", "examples."+dir.Name())
			convertDotnet(t, template, outDir)
			convertPython(t, template, outDir)
			convertNodeJS(t, template, outDir)
			convertGolang(t, template, outDir)
		})
	}
}

func TestGenerateProgram(t *testing.T) {
	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{
			{
				Directory:   "direct-invoke",
				Description: "Use an invoke directly",
			},
		}
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

// Tests both isEscapedString and asEscapedString.
func TestEscapedString(t *testing.T) {
	tests := []struct {
		input     string
		isEscaped bool   // If input is escaped
		asEscaped string // What input would look like when escaped
	}{
		{`"foobar"`, true, `"foobar"`},
		{`"foo\nbar"`, true, `"foo\nbar"`},
		{`"foo\"bar"`, true, `"foo\"bar"`},
		{`"foo\\\"bar"`, true, `"foo\\\"bar"`},
		{`"foo`, false, `"foo"`},
		{`"foo"bar"`, false, `"foo\"bar"`},
		{`"goo\\"bar"`, false, `"goo\\\"bar"`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.isEscaped, isEscapedString(tt.input))
			s := asEscapedString(tt.input)
			assert.Equal(t, tt.asEscaped, s)
			assert.True(t, isEscapedString(s), "A string should always be escaped after we escape it")
		})
	}
}
