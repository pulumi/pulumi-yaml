package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"
)

type mockResource struct {
	ID    string
	State map[string]interface{}
}

type mockCall struct {
	Return map[string]interface{}
}

type mocks struct {
	Resources map[string]mockResource // name -> mock
	Calls     map[string]mockCall     // token -> mock
}

func (m *mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	call, ok := m.Calls[args.Token]
	if !ok {
		return nil, fmt.Errorf("unexpected call to function '%v'", args.Token)
	}
	return resource.NewPropertyMapFromMap(call.Return), nil
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	res, ok := m.Resources[args.Name]
	if !ok {
		return "", nil, fmt.Errorf("unexpected resource '%v'", args.Name)
	}
	return res.ID, resource.NewPropertyMapFromMap(res.State), nil
}

type mockTemplate struct {
	Mocks *mocks `json:"mocks,omitempty" yaml:"mocks,omitempty"`
}

func loadMocks(bytes []byte) (*mocks, error) {
	var t mockTemplate
	if err := yaml.Unmarshal(bytes, &t); err != nil {
		return nil, err
	}
	return t.Mocks, nil
}

func runTemplate(path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	pt, diags, err := pulumiyaml.LoadYAMLBytes("<stdin>", source)
	if err != nil {
		return err
	}
	diagWriter := pt.NewDiagnosticWriter(os.Stderr, 0, true)
	if len(diags) != 0 {
		diagWriter.WriteDiagnostics(hcl.Diagnostics(diags))
	}

	mocks, err := loadMocks(source)
	if err != nil {
		return err
	}
	if mocks == nil {
		return errors.New("template must contain mocks")
	}

	err = pulumi.RunErr(func(ctx *pulumi.Context) error {
		return pulumiyaml.RunTemplate(ctx, pt)
	}, pulumi.WithMocks("foo", "dev", mocks))
	if diags, ok := pulumiyaml.HasDiagnostics(err); ok {
		diagWriter.WriteDiagnostics(hcl.Diagnostics(diags))
	}
	return err
}
