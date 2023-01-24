// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type testPackageLoader struct{ *testing.T }

func (l testPackageLoader) LoadPackage(name string, version *semver.Version) (pulumiyaml.Package, error) {
	if name == "test" {
		return FakePackage{l.T}, nil
	}

	pkg, err := schema.LoadPackageReference(rootPluginLoader, name, version)
	if err != nil {
		return nil, err
	}
	return pulumiyaml.NewResourcePackage(pkg), nil
}

func (l testPackageLoader) Close() {}

func newPluginLoader() schema.Loader {
	schemaLoadPath := filepath.Join("..", "testing", "test", "testdata")
	return schema.NewPluginLoader(utils.NewHost(schemaLoadPath))
}

var rootPluginLoader = newPluginLoader()

// We stub out the real plugin hosting architecture for a fake that gives us reasonably good
// results without the time and compute
type FakePackage struct {
	t *testing.T
}

const (
	OtherThing  = "other:index:Thing"
	OtherModule = "other:module:Object"
)

func (m FakePackage) ResolveResource(typeName string) (pulumiyaml.ResourceTypeToken, error) {
	switch typeName {
	case
		// TestImportTemplate fakes:
		"test:mod:prov", "test:mod:typ",
		// third-party-package fakes:
		OtherThing, OtherModule:
		return pulumiyaml.ResourceTypeToken(typeName), nil
	default:
		msg := fmt.Sprintf("Unexpected type token in ResolveResource: %q", typeName)
		m.t.Logf(msg)
		return "", fmt.Errorf(msg)
	}
}

func (m FakePackage) ResourceTypeHint(typeName pulumiyaml.ResourceTypeToken) *schema.ResourceType {
	switch typeName {
	case "test:mod:prov", "test:mod:typ":
		// third-party-package fakes:
		return &schema.ResourceType{Token: typeName.String()}
	case OtherThing:
		return &schema.ResourceType{
			Token: OtherThing,
			Resource: &schema.Resource{
				Token: OtherThing,
				InputProperties: []*schema.Property{
					{Name: "idea"},
				},
				Properties: []*schema.Property{
					{Name: "idea"},
				},
			},
		}
	case OtherModule:
		return &schema.ResourceType{
			Token: OtherModule,
			Resource: &schema.Resource{
				Token: OtherModule,
				InputProperties: []*schema.Property{
					{Name: "answer"},
				},
				Properties: []*schema.Property{
					{Name: "answer"},
				},
			},
		}

	default:
		return nil
	}
}

func (m FakePackage) ResourceConstants(typeName pulumiyaml.ResourceTypeToken) map[string]interface{} {
	return nil
}

func (m FakePackage) ResolveFunction(typeName string) (pulumiyaml.FunctionTypeToken, error) {
	switch typeName {
	case "test:mod:fn":
		return pulumiyaml.FunctionTypeToken(typeName), nil
	}
	msg := fmt.Sprintf("Unexpected type token in ResolveFunction: %q", typeName)
	m.t.Logf(msg)
	return "", fmt.Errorf(msg)
}

func (m FakePackage) FunctionTypeHint(typeName pulumiyaml.FunctionTypeToken) *schema.Function {
	return nil
}

func (m FakePackage) IsComponent(typeName pulumiyaml.ResourceTypeToken) (bool, error) {
	// No component test cases presently.
	// If the resource resolves, default to false until we add exceptions.
	if _, err := m.ResolveResource(string(typeName)); err == nil {
		// note this returns if err *equals* nil.
		return false, nil
	}
	msg := fmt.Sprintf("Unexpected type token in IsComponent: %q", typeName)
	m.t.Logf(msg)
	return false, fmt.Errorf(msg)
}

func (m FakePackage) Name() string {
	return "fake"
}

func (m FakePackage) Version() *semver.Version {
	return nil
}

//nolint:paralleltest // mutates environment variables
func TestGenerateProgram(t *testing.T) {
	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{
			{
				Directory:   "direct-invoke",
				Description: "Use an invoke directly",
			},
			{
				Directory:   "join-template",
				Description: "Converting a template expression into a join invoke",
			},
		}
		for _, tt := range tests {
			switch tt.Directory {
			case "synthetic-resource-properties":
				// https://github.com/pulumi/pulumi-yaml/issues/229
			case "azure-sa":
				// Reason: has dependencies between config variables
			case "aws-eks", "aws-s3-folder":
				// Reason: missing splat
				//
				// Note: aws-s3-folder errors with
				// 14,27-52: the asset parameter must be a string literal; the asset parameter must be a string literal
				// But the actual error is that it is using a Splat operator.
			case "python-resource-names":
				// Reason: A python only test.
			case "simple-range":
				// Pulumi YAML does not support ranges
			case "read-file-func", "python-regress-10914":
				tt.SkipCompile = codegen.NewStringSet("yaml")
				l = append(l, tt)
			case "traverse-union-repro":
				// Reason: this example is known to be invalid
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
			return pulumiyaml.RunTemplate(ctx, templateDecl, nil, testPackageLoader{t})
		}, pulumi.WithMocks("test", "gen", &testMonitor{}), func(ri *pulumi.RunInfo) { ri.DryRun = true })
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
	case "aws:iam/getPolicyDocument:getPolicyDocument":
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"json": `"some json"`,
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
	t.Parallel()

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
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.isEscaped, isEscapedString(tt.input))
			s := asEscapedString(tt.input)
			assert.Equal(t, tt.asEscaped, s)
			assert.True(t, isEscapedString(s), "A string should always be escaped after we escape it")
		})
	}
}

func TestCollapseToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
	}{
		{"aws:s3/bucket:Bucket", "aws:s3:Bucket"},
		{"foo:index:Bar", "foo:Bar"},
		{"fizz:mod:buzz", "fizz:mod:buzz"},
		{"aws:s3/buck:Bucket", "aws:s3/buck:Bucket"},
		{"too:many:semi:colons", "too:many:semi:colons"},
		{"foo:index/bar:Bar", "foo:Bar"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, collapseToken(tt.input))
		})
	}
}
