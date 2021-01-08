# PulumiYAML

An experimental YAML (and JSON) language provider for Pulumi.

## Examples

The Pulumi Getting Started:

```yaml
name: simple-yaml
runtime: yaml
resources: 
  my-bucket:
    type: aws:s3/bucket:Bucket
    properties:
      website:
        indexDocument: index.html
  index.html:
    type: aws:s3/bucketObject:BucketObject
    properties:
      bucket: !Ref my-bucket
      source:
        Fn::Asset:
          String: <h1>Hello, world!</h1>
      acl: public-read
      contentType: text/html
outputs:
  bucketEndpoint: http://${my-bucket.websiteEndpoint}
```

Webserver + kitchen sink (providers, config, resource options, invokes, interpolations):

```yaml
configuration:
  InstanceType:
    type: String
    default: t2.micro
    allowedValues:
      - t2.micro
      - m1.small
      - m1.large
    description: Enter t2.micro, m1.small, or m1.large. Default is t2.micro.
variables:
  AmazonLinuxAmi: 
    Fn::Invoke:
      Function: aws:index/getAmi:getAmi
      Arguments:
        filters:
          - name: name
            values: ["amzn-ami-hvm-*-x86_64-ebs"]
        owners: ["137112412989"]
        mostRecent: true
      Return: id
resources:
  WebSecGrp:
    type: aws:ec2/securityGroup:SecurityGroup
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
      instanceType: t2.micro
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
    type: aws:s3/bucket:Bucket
    provider: UsEast2Provider
outputs:
  InstanceId: ${WebServer}
  PublicIp: ${WebServer.publicIp}
  PublicHostName: ${WebServer.publicDns}
```
