// Copyright 2022, Pulumi Corporation.  All rights reserved.

package codegen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/stretchr/testify/assert"

	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type testPackageLoader struct{ *testing.T }

func (l testPackageLoader) LoadPackage(ctx context.Context, descriptor *schema.PackageDescriptor) (pulumiyaml.Package, error) {
	if descriptor.Name == "test" {
		return FakePackage{l.T}, nil
	}

	pkg, err := schema.LoadPackageReferenceV2(ctx, rootPluginLoader, descriptor)
	if err != nil {
		return nil, err
	}
	return pulumiyaml.NewResourcePackage(pkg), nil
}

func (l testPackageLoader) Close() {}

func newPluginContext() *plugin.Context {
	schemaLoadPath := filepath.Join("..", "testing", "test", "testdata")
	return utils.NewContextWithProviders(schemaLoadPath,
		utils.NewSchemaProvider("aws", "5.4.0"),
		utils.NewSchemaProvider("other", "0.1.0"),
		utils.NewSchemaProvider("using-dashes", "1.0.0"),
	)
}

func newPluginLoader() schema.Loader {
	return schema.NewPluginLoader(newPluginContext())
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
		m.t.Logf("%s", msg)
		return "", errors.New(msg)
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
	m.t.Logf("%s", msg)
	return "", errors.New(msg)
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
	m.t.Logf("%s", msg)
	return false, errors.New(msg)
}

func (m FakePackage) IsResourcePropertySecret(typeName pulumiyaml.ResourceTypeToken, propertyName string) (bool, error) {
	// No secret test cases presently.
	// If the resource resolves, default to false until we add exceptions.
	if _, err := m.ResolveResource(string(typeName)); err == nil {
		// note this returns if err *equals* nil.
		return false, nil
	}
	msg := fmt.Sprintf("Unexpected type token in IsComponent: %q", typeName)
	m.t.Logf("%s", msg)
	return false, errors.New(msg)
}

func (m FakePackage) Name() string {
	return "fake"
}

func (m FakePackage) Version() *semver.Version {
	return nil
}

func TestGenerateProgram(t *testing.T) {
	t.Parallel()
	filter := func(tests []test.ProgramTest) []test.ProgramTest {
		l := []test.ProgramTest{
			{
				Directory:     "direct-invoke",
				Description:   "Use an invoke directly",
				PluginContext: newPluginContext(),
			},
			{
				Directory:     "join-template",
				Description:   "Converting a template expression into a join invoke",
				PluginContext: newPluginContext(),
			},
			{
				Directory:     "aws-secret",
				Description:   "Keep secret() on a resource input",
				PluginContext: newPluginContext(),
			},
		}
		for _, tt := range tests {
			switch tt.Directory {
			case "components":
				// https://github.com/pulumi/pulumi-yaml/issues/476
			case "unknown-resource":
				// https://github.com/pulumi/pulumi-yaml/issues/478
			case "interpolated-string-keys":
				// https://github.com/pulumi/pulumi-yaml/issues/480
			case "throw-not-implemented":
				// Pulumi YAML does not support the notImplemented function.
			case "python-reserved", "snowflake-python-12998":
				// Reason: A python only test.
			case "invoke-inside-conditional-range":
				// Pulumi YAML does not support ranges
			case "read-file-func", "unknown-invoke":
				tt.SkipCompile = mapset.NewSet("yaml")
				l = append(l, tt)
			case "traverse-union-repro":
				// Reason: this example is known to be invalid
			default:
				l = append(l, tt)
			}
		}
		return l
	}

	check := func(t *testing.T, output string, _ mapset.Set[string]) {
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

	test.TestProgramCodegen(t, test.ProgramCodegenOptions{
		Language:   "yaml",
		Extension:  "yaml",
		OutputFile: "Main.yaml",
		Check:      check,
		GenProgram: GenerateProgram,
		TestCases: append(
			filter(test.PulumiPulumiProgramTests),
			test.ProgramTest{
				Directory:     "negative-literals",
				Description:   "Negative literals in Pulumi Programs",
				PluginContext: newPluginContext(),
			},
		),
	})
}

type testMonitor struct{}

func (m *testMonitor) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	if args.Token == "aws:iam/getPolicyDocument:getPolicyDocument" {
		return resource.NewPropertyMapFromMap(map[string]interface{}{
			"json": `"some json"`,
		}), nil
	}
	return resource.PropertyMap{}, nil
}

func (m *testMonitor) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
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
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, collapseToken(tt.input))
		})
	}
}
