// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"strings"
	"testing"

	"github.com/hexops/autogold"
	"github.com/pulumi/pulumi-yaml/pkg/pulumiyaml/packages"
	"github.com/stretchr/testify/assert"
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
                values: ["amzn2-ami-hvm-2.0.20231218.0-x86_64-ebs"]
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

	confNodes, err := getPulumiConfNodes(nil)
	assert.Nil(t, err)
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
	assert.Contains(t, diagString(diags[0]), "<stdin>:13:16: Package test already declared with a conflicting version: 1.23.425-beta.6")
	assert.Contains(t, diagString(diags[1]), "<stdin>:14:26: Package test already declared with a conflicting plugin download URL: https://example.com")
	assert.Empty(t, plugins)
}
