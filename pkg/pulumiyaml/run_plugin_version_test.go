// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

import (
	"strings"
	"testing"

	"github.com/hexops/autogold"
)

func TestVersionValueComplex(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    version: 1.23.425-beta.6
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins := GetReferencedPlugins(tmpl)

	got := plugins
	want := autogold.Want("test-plugins", []Plugin{{
		Package: "test",
		Version: "1.23.425-beta.6",
	}})
	want.Equal(t, got)

	diags := testTemplateSyntaxDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

func TestVersionValuePatched(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    version: 1.7.13
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins := GetReferencedPlugins(tmpl)

	got := plugins
	want := autogold.Want("test-plugins", []Plugin{{
		Package: "test",
		Version: "1.7.13",
	}})
	want.Equal(t, got)

	diags := testTemplateSyntaxDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

func TestVersionValueMajorMinor(t *testing.T) {
	const text = `
name: test-yaml
runtime: yaml
resources:
  res-a:
    type: test:resource:type
    version: "1.2"
    properties: {}
`

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins := GetReferencedPlugins(tmpl)

	got := plugins
	want := autogold.Want("test-plugins", []Plugin{{
		Package: "test",
		Version: "1.2",
	}})
	want.Equal(t, got)

	diags := testTemplateSyntaxDiags(t, tmpl, func(r *runner) {})
	requireNoErrors(t, diags)
}

func TestVersionOnExample(t *testing.T) {
	const text = `
configuration:
  InstanceType:
    type: String
    default: t2.micro
    allowedValues:
      - t2.micro
      - m1.small
      - m1.large
    description: Enter t2.micro, m1.small, or m1.large. Default is t2.micro.
resources:
  WebSecGrp:
    type: aws:ec2/securityGroup:SecurityGroup
    version: 4.37.1
    properties:
      ingress:
        - protocol: tcp
          fromPort: 80
          toPort: 80
          cidrBlocks: ["0.0.0.0/0"]
    protect: true
  WebServer:
    type: aws:ec2/instance:Instance
    properties:
      instanceType: ${InstanceType}
      ami:
        Fn::Invoke:
          Function: aws:index/getAmi:getAmi
          Arguments:
            filters:
              - name: name
                values: ["amzn-ami-hvm-*-x86_64-ebs"]
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
    provider: UsEast2Provider
outputs:
  InstanceId: ${WebServer}
  PublicIp: ${WebServer.publicIp}
  PublicHostName: ${WebServer.publicDns}
  `

	tmpl := yamlTemplate(t, strings.TrimSpace(text))
	plugins := GetReferencedPlugins(tmpl)

	gotPlugins := plugins
	wantPlugins := autogold.Want("test-plugins", []Plugin{
		{
			Package: "aws",
			Version: "4.37.1",
		},
		{Package: "aws"},
		{Package: "aws"},
		{Package: "aws"},
	})
	wantPlugins.Equal(t, gotPlugins)

	_, diags := topologicallySortedResources(tmpl)
	requireNoErrors(t, diags)
}
