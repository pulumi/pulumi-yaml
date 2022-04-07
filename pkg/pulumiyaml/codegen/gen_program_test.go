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
	"github.com/pulumi/pulumi/pkg/v3/resource/deploy/deploytest"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

var defaultPlugins []pulumiyaml.Plugin = []pulumiyaml.Plugin{
	{Package: "aws", Version: "4.26.0"},
	{Package: "azure-native", Version: "1.56.0"},
	{Package: "azure", Version: "4.18.0"},
	{Package: "kubernetes", Version: "3.7.2"},
	{Package: "random", Version: "4.2.0"},
	{Package: "eks", Version: "0.37.1"},
	{Package: "aws-native", Version: "0.13.0"},
	{Package: "docker", Version: "3.1.0"},

	// Extra packages are to satisfy the versioning requirement of aws-eks.
	// While the schemas are not the correct version, we rely on not
	// depending on the difference between them.
	{Package: "aws", Version: "4.15.0"},
	{Package: "kubernetes", Version: "3.0.0"},
}

type testPackageLoader struct{ *testing.T }

func (l testPackageLoader) LoadPackage(name string) (pulumiyaml.Package, error) {
	if name == "other" || name == "test" {
		return FakePackage{l.T}, nil
	}

	pkg, err := rootPluginLoader.LoadPackage(name, nil)
	if err != nil {
		return nil, err
	}
	return pulumiyaml.NewResourcePackage(pkg), nil
}

func (l testPackageLoader) Close() {}

func newPluginLoader() schema.Loader {
	schemaLoadPath := filepath.Join("..", "testing", "test", "testdata")
	host := func(pkg tokens.Package, version semver.Version) *deploytest.PluginLoader {
		return deploytest.NewProviderLoader(pkg, version, func() (plugin.Provider, error) {
			return utils.NewProviderLoader(pkg.String())(schemaLoadPath)
		})
	}
	var pluginLoaders []*deploytest.PluginLoader
	for _, p := range defaultPlugins {
		pluginLoaders = append(pluginLoaders, host(tokens.Package(p.Package), semver.MustParse(p.Version)))
	}

	return schema.NewPluginLoader(deploytest.NewPluginHost(nil, nil, nil, pluginLoaders...))
}

var rootPluginLoader schema.Loader = newPluginLoader()

// We stub out the real plugin hosting architecture for a fake that gives us reasonably good
// results without the time and compute
type FakePackage struct {
	t *testing.T
}

func (m FakePackage) ResolveResource(typeName string) (pulumiyaml.ResourceTypeToken, error) {
	switch typeName {
	case
		// TestImportTemplate fakes:
		"test:mod:prov", "test:mod:typ",
		// third-party-package fakes:
		"other:index:Thing", "other:module:Object":
		return pulumiyaml.ResourceTypeToken(typeName), nil
	default:
		msg := fmt.Sprintf("Unexpected type token in ResolveResource: %q", typeName)
		m.t.Logf(msg)
		return "", fmt.Errorf(msg)
	}
}

func (m FakePackage) ResourceTypeHint(typeName pulumiyaml.ResourceTypeToken) pulumiyaml.ResourceTypeHint {
	switch typeName {
	case "test:mod:prov", "test:mod:typ",
		// third-party-package fakes:
		"other:index:Thing", "other:module:Object":
		return FakeTypeHint{typeName}
	}
	return nil
}

func (m FakePackage) ResolveFunction(typeName string) (pulumiyaml.FunctionTypeToken, error) {
	msg := fmt.Sprintf("Unexpected type token in ResolveFunction: %q", typeName)
	m.t.Logf(msg)
	return "", fmt.Errorf(msg)
}

func (m FakePackage) FunctionTypeHint(typeName pulumiyaml.FunctionTypeToken) pulumiyaml.TypeHint {
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

type FakeTypeHint struct {
	resourceName pulumiyaml.ResourceTypeToken
}

func (frp FakeTypeHint) Fields() map[string]pulumiyaml.TypeHint {
	return nil
}
func (frp FakeTypeHint) Element() pulumiyaml.TypeHint {
	return nil
}
func (frp FakeTypeHint) InputProperties() map[string]pulumiyaml.TypeHint {
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
			return pulumiyaml.RunTemplate(ctx, templateDecl, testPackageLoader{t})
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
