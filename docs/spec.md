# Pulumi YAML Specification

Pulumi YAML will be defined as part of the `Pulumi.yaml` file. In addition to the existing `Pulumi.yaml`, there will be 4(5?) additional top-level sections. Each section is discussed in detail below.

| Property      | Type | Required           | Expression  | Description |
| ------------- |------|-------------| -----|---|
| `packages`    | map[string]Package (To discuss)       | No | No | packages specifies the pulumi packages (and versions) to be used in the program. |
| `config`      | map[string]Configuration | No | No | config specifies the [Pulumi config](https://www.pulumi.com/docs/intro/concepts/config/) inputs to the deployment. |
| (?) `variables`| To discuss | To discuss | To discuss | To discuss |
| `resources`   | map[string]Resource      | No | No | resources declares the [Pulumi resources](https://www.pulumi.com/docs/intro/concepts/resources/) that will be deployed and managed by the program |
| `outputs`     | map[string]Expression    | No | Yes | outputs specifies the [Pulumi stack outputs](https://www.pulumi.com/docs/intro/concepts/stack/#outputs) of the program and how they are computed from the `resources` is a value of the appropriate type for the template to use if no value is specified. |

## Packages

The Packages section specifies the pulumi packages (and versions) to be used in the program. 

If no packages section is specified, or if a resource is specified but the related package is missing from the package list, the package name will be determined from the resource's type token and the `latest` version will be used.

| Property      |  Type  | Required | Expression  | Description |
| ------------- |--------|----------| ------------|-------------|
| `version`     | string | No       | No          | The version of the package to use in the program. |

## Config

The value of `config` is an object whose keys are logical names by which the config input will be referenced in expressions within the program, and whose values are elements of the schema below. Each key in this object represents an independent config input.

| Property      | Type   | Required | Expression  | Description |
| ------------- |--------|----------| -----|---|
| `type`        | string | Yes      | No | Type is the (required) data type for the parameter. It can be one of: `String`, `Number`, `List<Number>`, or `CommaDelimitedList`. |
| `default`     | any    | No       | No | Default is a value of the appropriate type for the template to use if no value is specified. |
| `secret`      | bool   | No       | No | Secret masks the parameter by marking it a secret. |

### Questions

* Are these the right types? I kept what was in the README but should this reflect the Pulumi config types?

## Resources

The value of `resources` is an object whose keys  are logical resource names by which the resource will be referenced in expressions within the program, and whose values are elements of the schema below.  Each item in this object represents a resource which will be managed by the Pulumi program.

| Property          | Type | Required           | Expressions  | Description |
| ----------------- |---|-------------| -----|---|
| `type`            | string | Yes | No | Type is the Pulumi type token for this resource. |
| `properties`      | map[string]Expression | No | Yes | Properties contains the primary resource-specific keys and values to initialize the resource state. |
| `additionalSecretOutputs`      | string[] | No | No | AdditionalSecretOutputs specifies properties that must be encrypted as secrets |
| `aliases`      | string[] | No | No | Aliases specifies names that this resource used to have so that renaming or refactoring doesn’t replace it |
| `customTimeouts`      | CustomTimeout | No | No | CustomTimeouts overrides the default retry/timeout behavior for resource provisioning |
| `deleteBeforeReplace`      | bool | No | No | DeleteBeforeReplace  overrides the default create-before-delete behavior when replacing |
| `dependsOn`      | string[] | No | No | DependsOn makes this resource explicitly depend on another resource, by name, so that it won't be created before the dependent finishes being created (and the reverse for destruction). Normally, Pulumi automatically tracks implicit dependencies through inputs/outputs, but this can be used when dependencies aren't captured purely from input/output edges.|
| `ignoreChanges`      | string[] | No | No | IgnoreChanges declares that changes to certain properties should be ignored during diffing |
| `import`      | string | No | No | Import adopts an existing resource from your cloud account under the control of Pulumi |
| `parent`      | string | No | No | Parent specifies a parent for the resource |
| `protect`      | bool | No | No | Protect prevents accidental deletion of a resource |
| `provider`      | string | No | No | Provider specifies an explicitly configured provider, instead of using the default global provider |

## Outputs

The value of `outputs` is an object whose keys are the logical names of the outputs that are available from outside the Pulumi stack (via `pulumi stack output`). Its values are potentially computed expressions that resolve to the values of the desired outputs.

## Expressions

Expressions can be used in two contexts: 

1. the values of `properties` of `resources`
2. the values of `outputs`

In expression locations, strings are evaluated as interpolations and any nested `${...}` expressions within the string value are replaced by the value of the expression `...`.  The syntax of expressions within interpolations is:
```
expr           := [ expr '.' ] identifier
identifier     := letter ( letter | unicode_digit )*
letter         := ( unicode_letter | "_" )*

unicode_letter is a Unicode code point classified as "Letter"
unicode_digit  is a Unicode code point classified as "Number, decimal digit"
```


