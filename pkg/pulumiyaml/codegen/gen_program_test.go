// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// We stub out the real plugin hosting architecture for a fake that gives us reasonably good
// results without the time and compute
type FakePackage struct {
	t *testing.T
}

func (m FakePackage) ResolveResource(typeName string) (pulumiyaml.CanonicalTypeToken, error) {
	switch typeName {
	case "aws:ec2/instance:Instance",
		"aws:ec2/networkAcl:NetworkAcl",
		"aws:ec2/networkAclRule:NetworkAclRule",
		"aws:ec2/securityGroup:SecurityGroup",
		"aws:ec2/vpc:Vpc",
		"aws:ec2/vpcEndpoint:VpcEndpoint",
		"aws:ecs/cluster:Cluster",
		"aws:ecs/service:Service",
		"aws:ecs/taskDefinition:TaskDefinition",
		"aws:elasticloadbalancingv2/listener:Listener",
		"aws:elasticloadbalancingv2/loadBalancer:LoadBalancer",
		"aws:elasticloadbalancingv2/targetGroup:TargetGroup",
		"aws:iam/policy:Policy",
		"aws:iam/role:Role",
		"aws:iam/rolePolicyAttachment:RolePolicyAttachment",
		"aws:rds/cluster:Cluster",
		"aws:s3/bucket:Bucket",
		"azure-native:cdn:Endpoint",
		"azure-native:network:FrontDoor",
		"kubernetes:apps/v1:Deployment",
		"kubernetes:core/v1:Pod",
		"kubernetes:core/v1:ServiceAccount",
		"kubernetes:rbac.authorization.k8s.io/v1:Role",
		"kubernetes:rbac.authorization.k8s.io/v1:RoleBinding",
		"other:index:Thing",
		"other:module:Object",
		"pulumi:providers:aws",
		"random:index/randomPet:RandomPet":
		return pulumiyaml.CanonicalTypeToken(typeName), nil
	// TestImportTemplate fakes:
	case "test:mod:prov", "test:mod:typ":
		return pulumiyaml.CanonicalTypeToken(typeName), nil
	default:
		msg := fmt.Sprintf("Unexpected type token in ResolveResource: %q", typeName)
		m.t.Logf(msg)
		return "", fmt.Errorf(msg)
	}
}

func (m FakePackage) ResolveFunction(typeName string) (pulumiyaml.CanonicalTypeToken, error) {
	switch typeName {
	case "aws:ec2/getVpc:getVpc",
		"aws:ec2/getSubnetIds:getSubnetIds",
		"aws:iam/getPolicyDocument:getPolicyDocument",
		"aws:index/getAmi:getAmi",
		"aws:ec2/getPrefixList:getPrefixList",
		"aws:ec2/getAmiIds:getAmiIds":
		return pulumiyaml.CanonicalTypeToken(typeName), nil
	default:
		msg := fmt.Sprintf("Unexpected type token in ResolveFunction: %q", typeName)
		m.t.Logf(msg)
		return "", fmt.Errorf(msg)
	}
}

func (m FakePackage) IsComponent(typeName pulumiyaml.CanonicalTypeToken) (bool, error) {
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

func newFakePackageMap(t *testing.T) pulumiyaml.PackageMap {
	return pulumiyaml.PackageMap{
		"aws":          FakePackage{t},
		"azure-native": FakePackage{t},
		"kubernetes":   FakePackage{t},
		"other":        FakePackage{t},
		"random":       FakePackage{t},
		"test":         FakePackage{t},
	}
}

func TestGenerateProgram(t *testing.T) {
	skipNonexistantPackageTest := false
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
			case "third-party-package":
				if !skipNonexistantPackageTest {
					// Reason: when we run the full aka integration test, we allow Pulumi to host the plugin
					// packages and perform real schema resolution. This test uses a fake package named
					// "other".
					l = append(l, tt)
				}
			default:
				l = append(l, tt)
			}
		}
		return l
	}

	var packageMap pulumiyaml.PackageMap
	if testing.Short() {
		packageMap = newFakePackageMap(t)
	} else {
		var err error
		plugins := []pulumiyaml.Plugin{
			{Package: "aws"},
			{Package: "kubernetes"},
			{Package: "random"},
			{Package: "azure-native"},
		}
		pluginCtx, pkgMap, err := pulumiyaml.NewResourcePackageMap(plugins)
		t.Cleanup(func() { pluginCtx.Close() })
		assert.NoError(t, err)
		if err != nil {
			t.FailNow()
		}
		packageMap = pkgMap
		skipNonexistantPackageTest = true
	}

	check := func(t *testing.T, output string, _ codegen.StringSet) {
		file, err := os.ReadFile(output)
		assert.NoError(t, err)
		templateDecl, diags, err := pulumiyaml.LoadYAMLBytes(output, file)
		assert.NoError(t, err)
		assert.Falsef(t, diags.HasErrors(), "%s", diags.Error())
		err = pulumi.RunErr(func(ctx *pulumi.Context) error {
			return pulumiyaml.RunTemplate(ctx, templateDecl, packageMap)
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
