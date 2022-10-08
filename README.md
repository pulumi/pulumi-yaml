# PulumiYAML

A YAML (and JSON) language provider for Pulumi.

## Preview

To use YAML language support in Pulumi, either:

* Clone this repo and run `make install` to build from source.

* Download the [latest release](https://github.com/pulumi/pulumi-yaml/releases) for your platform
  and place `pulumi-language-yaml` on your PATH. This can be in ~/.pulumi/bin or any other location.

* (Requires Pulumi 3.27.0) Configure a [GitHub personal access
  token](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/creating-a-personal-access-token)
  with the "repo" scope and set the GITHUB_TOKEN environment variable to your token before running:

  ```
  pulumi plugin install language yaml
  ```

## Examples

See [examples dir](./examples) for tested examples.

The Pulumi Getting Started:

```yaml
name: simple-yaml
runtime: yaml
resources:
  my-bucket:
    type: aws:s3:Bucket
    properties:
      website:
        indexDocument: index.html
  index.html:
    type: aws:s3:BucketObject
    properties:
      bucket: ${my-bucket}
      source:
        fn::stringAsset: <h1>Hello, world!</h1>
      acl: public-read
      contentType: text/html
outputs:
  bucketEndpoint: http://${my-bucket.websiteEndpoint}
```

Webserver + kitchen sink (providers, config, resource options, invokes, interpolations):

```yaml
name: webserver
runtime: yaml
description: Basic example of an AWS web server accessible over HTTP
configuration:
  InstanceType:
    default: t3.micro
variables:
  AmazonLinuxAmi:
    fn::invoke:
      function: aws:getAmi
      arguments:
        filters:
          - name: name
            values: ["amzn-ami-hvm-*-x86_64-ebs"]
        owners: ["137112412989"]
        mostRecent: true
      return: id
resources:
  WebSecGrp:
    type: aws:ec2:SecurityGroup
    properties:
      ingress:
        - protocol: tcp
          fromPort: 80
          toPort: 80
          cidrBlocks: ["0.0.0.0/0"]
    protect: true
  WebServer:
    type: aws:ec2:Instance
    properties:
      instanceType: ${InstanceType}
      ami: ${AmazonLinuxAmi}
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
    type: aws:s3:Bucket
    options:
      provider: ${UsEast2Provider}
outputs:
  InstanceId: ${WebServer.id}
  PublicIp: ${WebServer.publicIp}
  PublicHostName: ${WebServer.publicDns}
```

## Spec

The specification for the Pulumi YAML format, and documentation for built-in functions, is in the
[Pulumi YAML reference](https://pulumi.com/docs/reference/yaml).

Contribute to the specification by editing [the markdown file in
pulumi/pulumi-hugo](https://github.com/pulumi/pulumi-hugo/blob/master/themes/default/content/docs/intro/languages/yaml.md).
