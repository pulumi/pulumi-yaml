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
  index.html:
    type: aws:s3:BucketObject
    properties:
      bucket: ${my-bucket}
      source:
        Fn::StringAsset: <h1>Hello, world!</h1>
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
    type: String
    default: t3.micro
    allowedValues:
      - t3.micro
      - t3.small
      - m4.large
    description: Enter t3.micro, t3.small, or m4.large. Default is t3.micro.
variables:
  AmazonLinuxAmi:
    Fn::Invoke:
      Function: aws:getAmi
      Arguments:
        filters:
          - name: name
            values: ["amzn-ami-hvm-*-x86_64-ebs"]
        owners: ["137112412989"]
        mostRecent: true
      Return: id
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

Pulumi programs can be defined in many languages, and the Pulumi YAML dialect offers an additional language for authoring Pulumi programs.

The Pulumi YAML provider supports programs written in YAML or JSON.  In both cases, the programs (`.yaml` or `.json` files) follow a simple schema, including four top level sections:

| Property        | Type | Required           | Expression  | Description |
| ------------- |---|-------------| -----|---|
| `configuration`      | map[string]Configuration | No | No | Configuration specifies the [Pulumi config](https://www.pulumi.com/docs/intro/concepts/config/) inputs to the deployment. |
| `resources`      | map[string]Resource | No | No | Resources declares the [Pulumi resources](https://www.pulumi.com/docs/intro/concepts/resources/) that will be deployed and managed by the program |
| `outputs`      | map[string]Expression | No | Yes | Outputs specifies the [Pulumi stack outputs](https://www.pulumi.com/docs/intro/concepts/stack/#outputs) of the program and how they are computed from the `resources` is a value of the appropriate type for the template to use if no value is specified. |

In many locations within this schema, values may be expressions which computed a value based on the `configuration` or outputs of `resources`.  These expressions can be provided in two ways:

* If an object is provided as a value, and has a key that is `Ref` or has the prefix `Fn::`, the object is treated as an expression, and the expression will be resolved to a new value that will be used in place of the object.
* Any string value is interpreted as an interpolation, with `${...}` being replaced by evaluating the expression in the `...`.

The supported expression forms for each of these is detailed below.

### Configuration

The value of `configuration` is an object whose keys are logical names by which the config input will be referenced in expressions within the program, and whose values are elements of the schema below.  Each item in this object represents an independent config input.

| Property        | Type | Required           | Expression  | Description |
| ------------- |---|-------------| -----|---|
| `type`      | string | Yes | No | Type is the (required) data type for the parameter. It can be one of: `String`, `Number`, `List<Number>`, or `CommaDelimetedList`. |
| `default`      | any | No | No | Default is a value of the appropriate type for the template to use if no value is specified. |
| `default`      | any | No | No | Default is a value of the appropriate type for the template to use if no value is specified. |
| `secret`      | bool | No | No | Secret masks the parameter by marking it a secret. |

### Resources

The value of `resources` is an object whose keys  are logical resource names by which the resource will be referenced in expressions within the program, and whose values which are elements of the schema below.  Each item in this object represents a resource which will be managed by the Pulumi program.

| Property        | Type | Required           | Expressions  | Description |
| ------------- |---|-------------| -----|---|
| `type`      | string | Yes | No | Type is the Pulumi type token for this resource. |
| `properties`      | map[string]Expression | No | Yes | Properties contains the primary resource-specific keys and values to initialize the resource state. |
| `additionalSecretOutputs`      | string[] | No | No | AdditionalSecretOutputs specifies properties that must be encrypted as secrets |
| `aliases`      | string[] | No | No | Aliases specifies names that this resource used to be have so that renaming or refactoring doesn’t replace it |
| `customTimeouts`      | CustomTimeout | No | No | CustomTimeouts overrides the default retry/timeout behavior for resource provisioning |
| `deleteBeforeReplace`      | bool | No | No | DeleteBeforeReplace  overrides the default create-before-delete behavior when replacing |
| `dependsOn`      | string[] | No | Yes | DependsOn makes this resource explicitly depend on another resource, by name, so that it won't be created before the dependent finishes being created (and the reverse for destruction). Normally, Pulumi automatically tracks implicit dependencies through inputs/outputs, but this can be used when dependencies aren't captured purely from input/output edges.|
| `ignoreChanges`      | string[] | No | No | IgnoreChangs declares that changes to certain properties should be ignored during diffing |
| `import`      | string | No | No | Import adopts an existing resource from your cloud account under the control of Pulumi |
| `parent`      | string | No | Yes | Parent specifies a parent for the resource |
| `protect`      | bool | No | No | Protect prevents accidental deletion of a resource |
| `provider`      | string | No | Yes | Provider specifies an explicitly configured provider, instead of using the default global provider |
| `providers`      | map[string]Expression | No | Yes | Map of providers for a resource and its children. |
| `version`      | string | No | No | Version specifies a provider plugin version that should be used when operating on a resource |

#### CustomTimeout

The optional `customTimeouts` property of a resource is an object of the following schema:

| Property        | Type | Required           | Expression  | Description |
| ------------- |---|-------------| -----|---|
| `create`      | string | No | No | Create is the custom timeout for create operations. |
| `delete`      | string | No | No | Delete is the custom timeout for delete operations. |
| `update`      | string | No | No | Update is the custom timeout for update operations. |

### Outputs

The value of `outputs` is an object whose keys are the logical names of the outputs that are available from outside the Pulumi stack (via `pulumi stack output`), and whose values are potentially computed expressions the resolve to the values of the desired outputs.

### Expressions

Expressions can be used in several contexts:
* the values of `properties` of `resources`
* the values of `options` of `resources` that take references to other resources:
* the values of `variables` and `outputs`

In these contexts, any JSON/YAML value may be provided.  If that value is a string, it is interpolated.  If that value is an object, and the object has a key with the name `Ref` or with a prefix of `Fn::`, it is evaluated as an expression.

#### Interpolation

In expression locations, strings are evaluated as interpolations and any nested `${...}` expressions within the string value are replaced by the value of the expression `...`.  The syntax of expressions within interpolations is:

```
expr           := [ expr '.' ] identifier
identifier     := letter ( letter | unicode_digit )*
letter         := ( unicode_letter | "_" )*

unicode_letter is a Unicode code point classified as "Letter"
unicode_digit  is a Unicode code point classified as "Number, decimal digit"
```

#### Built-in Functions

In any expression location, an object containing a single key beginning with "Fn::" calls a built-in function.

##### `Fn::Invoke`

Calls a function from a package and returns either the whole object or a single key if given the "Return" property. The schema is:

| Property        | Type | Required           | Expression  | Description |
| ------------- |---|-------------| -----|---|
| `Function`    | string | Yes | No | Name of a function to call. |
| `Arguments`   | map[string]Expression | Yes | Yes | Arguments to pass to the expression, each key is a named argument. |
| `Return`      | string | No | No | If the function returns an object, a single key may be selected and returned instead with its name. |

```yaml
variables:
  AmazonLinuxAmi:
    Fn::Invoke:
      Function: aws:getAmi
      Arguments:
        filters:
          - name: name
            values: ["amzn-ami-hvm-*-x86_64-ebs"]
        owners: ["137112412989"]
        mostRecent: true
      Return: id
```

The expression `${AmazonLinuxAmi}` will return the AMI ID returned from the [`aws:getAmi`](https://www.pulumi.com/registry/packages/aws/api-docs/getami/) function.

##### `Fn::Join`

Joins strings together separated by a delimiter. Arguments are passed as a list, with the first item being the delimiter, and the second item a list of expressions to concatenate.

```yaml
variables:
    banana:
        Fn::Join:
            - 'NaN'
            - - Ba
              - a
```

The expression `${banana}` will have the value "BaNaNa".

##### `Fn::Select`

Selects one of several options given an index. Arguments are passed as a list, with the first item being the index, 0-based, and the second item a list of expressions to select from.


```yaml
variables:
    policyVersion:
        Fn::Select:
            - 1
            - - v1
              - v1.1
              - v2.0
```

The expression `${policyVersion}` will have the value `v1.1`.

##### `Fn::*Asset` and `Fn::*Archive`

[Assets and Archives](https://www.pulumi.com/docs/intro/concepts/assets-archives/) are intrinsic types to Pulumi, like strings and numbers, and some resources may take these as inputs or return them as outputs. The built-ins create each kind of asset or archive. Each takes all take a single string value.


| Built-In      | Argument Type | Description |
| ------------- |---|------|
| `Fn::FileAsset` | string | The contents of the asset are read from a file on disk. |
| `Fn::StringAsset` | string | The contents of the asset are read from a string in memory. |
| `Fn::RemoteAsset` | string | The contents of the asset are read from an http, https or file URI. |
| `Fn::FileArchive` | string | The contents of the archive are read from either a folder on disk or a file on disk in one of the supported formats: .tar, .tgz, .tar.gz, .zip or .jar. |
| `Fn::RemoteArchive` | string | The contents of the asset are read from an http, https or file URI, which must produce an archive of one of the same supported types as FileArchive. |
| `Fn::AssetArchive` | map | The contents of the archive are read from a map of either Asset or Archive objects, one file or folder respectively per entry in the map.


```yaml
variables:
  aFile:
    Fn::FileAsset: ./file.txt
  aString:
    Fn::StringAsset: Hello, world!
  aRemoteAsset:
    Fn::RemoteAsset: http://worldclockapi.com/api/json/est/now

  aFileArchive:
    Fn::FileArchive: ./file.zip
  aRemoteArchive:
    Fn::RemoteArchive: http://example.com/file.zip
  anAssetArchive:
    Fn::AssetArchive:
      file:
        Fn::StringAsset: Hello, world!
      folder:
        Fn::FileArchive: ./folder
```

##### `Fn::StackReference`

[Stack References](https://www.pulumi.com/docs/intro/concepts/stack/#stackreferences) allow accessing the outputs of a stack from a YAML program. Arguments are passed as a list, with the first item being the stack name and the second argument the name of an output to reference:

```yaml
variables:
  reference:
    Fn::StackReference:
      - org/project/stack
      - outputName
```

The expression `${reference}` will have the value of the `outputName` output from the stack `org/project/stack`.
