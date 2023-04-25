# PulumiYAML

A YAML (and JSON) language provider for Pulumi.

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
  # The ownershipControls and publicAccessBlock resources are required as of April 2023
  # https://aws.amazon.com/blogs/aws/heads-up-amazon-s3-security-changes-are-coming-in-april-of-2023/
  ownershipControls:
    type: aws:s3:BucketOwnershipControls
    properties:
      bucket: ${my-bucket}
      rule:
        objectOwnership: ObjectWriter
  publicAccessBlock:
    type: aws:s3:BucketPublicAccessBlock
    properties:
      bucket: ${my-bucket}
      blockPublicAcls: false
  index.html:
    type: aws:s3:BucketObject
    properties:
      bucket: ${my-bucket}
      source:
        fn::stringAsset: <h1>Hello, world!</h1>
      acl: public-read
      contentType: text/html
    options:
      dependsOn:
        - ${ownershipControls}
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
