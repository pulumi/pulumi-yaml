// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"strings"
	"testing"

	"github.com/hexops/autogold"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/packages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionValueComplex(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    options:
      version: 1.23.425-beta.6
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins, diags := GetReferencedPackages(tmpl)
	assert.False(t, diags.HasErrors())

	got := plugins
	want := autogold.Want("test-plugins", []packages.PackageDecl{{
		Name:    "test",
		Version: "1.23.425-beta.6",
	}})
	want.Equal(t, got)

	diags = testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestVersionValuePatched(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    options:
      version: 1.7.13
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins, diags := GetReferencedPackages(tmpl)
	assert.False(t, diags.HasErrors())

	got := plugins
	want := autogold.Want("test-plugins", []packages.PackageDecl{{
		Name:    "test",
		Version: "1.7.13",
	}})
	want.Equal(t, got)

	diags = testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestVersionValueMajorMinor(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    options:
      version: "1.2"
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins, diags := GetReferencedPackages(tmpl)
	assert.False(t, diags.HasErrors())

	got := plugins
	want := autogold.Want("test-plugins", []packages.PackageDecl{{
		Name:    "test",
		Version: "1.2",
	}})
	want.Equal(t, got)

	diags = testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestVersionOnExample(t *testing.T) {
	t.Parallel()

	const text = `
configuration:
  InstanceType:
    type: String
    default: t2.micro
resources:
  WebSecGrp:
    type: aws:ec2/securityGroup:SecurityGroup
    options:
      version: 4.37.1
      protect: true
    properties:
      ingress:
        - protocol: tcp
          fromPort: 80
          toPort: 80
          cidrBlocks: ["0.0.0.0/0"]
  WebServer:
    type: aws:ec2/instance:Instance
    properties:
      instanceType: ${InstanceType}
      ami:
        fn::invoke:
          function: aws:ec2:getAmi
          arguments:
            filters:
              - name: name
                values: ["amzn2-ami-hvm-*-x86_64-ebs"]
            owners: ["137112412989"]
            mostRecent: true
          Return: id
      userData: |-
          #!/bin/bash
          echo 'Hello, World from ${WebSecGrp.arn}!' > index.html
          nohup python -m SimpleHTTPServer 80 &
      vpcSecurityGroupIds:
        - ${WebSecGrp}
  UsEast2Provider:
    type: pulumi:providers:aws
    properties:
      region: us-east-2
  MyBucket:
    type: aws:s3/bucket:Bucket
    options:
      provider: ${UsEast2Provider}
outputs:
  InstanceId: ${WebServer}
  PublicIp: ${WebServer.publicIp}
  PublicHostName: ${WebServer.publicDns}
  `

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins, diags := GetReferencedPackages(tmpl)
	assert.False(t, diags.HasErrors())

	gotPlugins := plugins
	wantPlugins := autogold.Want("test-plugins", []packages.PackageDecl{
		{
			Name:    "aws",
			Version: "4.37.1",
		},
	})
	wantPlugins.Equal(t, gotPlugins)

	confNodes := []configNode{}
	_, diags = topologicallySortedResources(tmpl, confNodes)
	requireNoErrors(t, tmpl, diags)
}

func TestVersionDuplicate(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    options:
      version: 1.23.425-beta.6
      pluginDownloadURL: https://example.com
    properties: {}
  res-b:
    type: test:resource:type
    options:
      version: 1.23.425-beta.6
      pluginDownloadURL: https://example.com
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins, diags := GetReferencedPackages(tmpl)
	assert.False(t, diags.HasErrors())

	got := plugins
	want := autogold.Want("test-plugins", []packages.PackageDecl{{
		Name:        "test",
		Version:     "1.23.425-beta.6",
		DownloadURL: "https://example.com",
	}})
	want.Equal(t, got)

	diags = testTemplateSyntaxDiags(t, tmpl, func(r *Runner) {})
	requireNoErrors(t, tmpl, diags)
}

func TestVersionConflicts(t *testing.T) {
	t.Parallel()

	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    options:
      version: 1.23.425-beta.6
      pluginDownloadURL: https://example.com
    properties: {}
  res-b:
    type: test:resource:type
    options:
      version: '2.0'
      pluginDownloadURL: https://example.com/v2
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins, diags := GetReferencedPackages(tmpl)
	assert.Empty(t, diags)
	// Per-resource version/URL are runtime directives. The first resource's
	// version is used when there is no SDK declaration.
	require.Len(t, plugins, 1)
	assert.Equal(t, "test", plugins[0].Name)
	assert.Equal(t, "1.23.425-beta.6", plugins[0].Version)
	assert.Equal(t, "https://example.com", plugins[0].DownloadURL)
}
