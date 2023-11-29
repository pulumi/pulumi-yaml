CHANGELOG
=========

## 1.4.3 (2023-11-29)

### Bug Fixes

- Fixes StackReference resources so that they are always read.
  [#529](https://github.com/pulumi/pulumi-yaml/pull/529)

## 1.4.2 (2023-11-17)

### Bug Fixes

- Fields marked as secret in the provider schema are now correctly
  handled as secrets. [#526](https://github.com/pulumi/pulumi-yaml/pull/526)

## 1.4.1 (2023-11-10)

### Bug Fixes

- Fix a regression where secret values were not handled correctly.
  [#519](https://github.com/pulumi/pulumi-yaml/pull/519)

## 1.4.0 (2023-10-27)

### Improvements

- Update pulumi/pulumi to v3.78.1
  [#493](https://github.com/pulumi/pulumi-yaml/pull/493)

- Publish pulumi-converter-yaml.

- Plugins: clean up resources and exit cleanly on receiving SIGINT or CTRL_BREAK.

- Improve handling of types of config fields.

### Bug Fixes

- Allow `protect` resource option to be set dynamically.


## 1.3.0 (2023-08-29)

### Improvements

- Update pulumi/pulumi to v3.78.1
  [#493](https://github.com/pulumi/pulumi-yaml/pull/493)

- Publish pulumi-converter-yaml.

- Plugins: clean up resources and exit cleanly on receiving SIGINT or CTRL_BREAK.

## 1.2.1 (2023-08-10)

### Bug Fixes

- Return a useful error message when a resource does not have a 'type' field
  specified, rather than a panic.
  [#468](https://github.com/pulumi/pulumi-yaml/pull/468)

- Fix nested access of unknown properties.
  [#490](https://github.com/pulumi/pulumi-yaml/pull/490)

## 1.2.0 (2023-08-08)

### Improvements

- Pre-built binaries of pulumi-language-yaml are now built with Go 1.20.
- Upgrade to Pulumi v3.76.
- Fix unknown resource outputs causing the program to fail during preview.

## 1.1.0 (2023-04-06)

### Improvements

- Add support for the `deletedWith` resource option.
  [#437](https://github.com/pulumi/pulumi-yaml/pull/437)

### Bug Fixes

- Avoid panicing for non-string map keys
  [#428](https://github.com/pulumi/pulumi-yaml/pull/428)

## 1.0.4 (2022-12-07)

### Improvements

- Deprecate `fn::stackReference`.
  [#420](https://github.com/pulumi/pulumi-yaml/pull/420)

- Ensure resource and invoke option "version" is used in package resolution, enabling Docker v4
  provider `docker:Image` resource.
  [#423](https://github.com/pulumi/pulumi-yaml/pull/423)

## 1.0.3 (2022-11-17)

### Improvements

- Parse `config` block for `pulumi convert`.
  [#407](https://github.com/pulumi/pulumi-yaml/pull/407)

### Bug Fixes

## 1.0.2 (2022-11-08)

### Improvements

### Bug Fixes

- Do not error on duplicate config keys.
  [#402](https://github.com/pulumi/pulumi-yaml/pull/402)

## 1.0.1 (2022-11-02)

### Improvements

- Allow ejecting when relying on `config` nodes.
  [#393](https://github.com/pulumi/pulumi-yaml/pull/393)

## 1.0.0 (2022-11-02)

### Improvements

- Ensure that constant values passed to enums are valid.
  [#357](https://github.com/pulumi/pulumi-yaml/pull/357)

- Warn on non camelCase names.
  [#362](https://github.com/pulumi/pulumi-yaml/pull/362)

- Recognize the new core project-level `config` block.
  [#369](https://github.com/pulumi/pulumi-yaml/pull/369)

### Bug Fixes

- Allow interpolations for `AssetOrArchive` function values
  [#341](https://github.com/pulumi/pulumi-yaml/pull/341)

- Clarify the lifetimes when calling `codegen.Eject`. This is a breaking change to the
  `codegen.Eject` API.
  [#358](https://github.com/pulumi/pulumi-yaml/pull/358)

- Quote generated strings that could be numbers.
  [#363](https://github.com/pulumi/pulumi-yaml/issues/363)

- Respect import option on resource.
  [#367](https://github.com/pulumi/pulumi-yaml/issues/367)

- Discover Invokes during `GetReferencedPlugins`.
  [#381](https://github.com/pulumi/pulumi-yaml/pull/381)

- Escaped interpolated strings now remove one extra dollar sign.
  [#382](https://github.com/pulumi/pulumi-yaml/pull/382)

- Only insert "id" in ejected resource refs when the receiver type is a string.
  [#389](https://github.com/pulumi/pulumi-yaml/pull/389)

## 0.5.3 (2022-07-12)

### Improvements

- Allow lowercase `type` on config entries.
  [#275](https://github.com/pulumi/pulumi-yaml/pull/275)

- Prevent errors from cascading.
  [#258](https://github.com/pulumi/pulumi-yaml/pull/258)

- Health checks for pulumi-language-yaml.
  [#277](https://github.com/pulumi/pulumi-yaml/pull/277)
- Move Pulumi YAML specification to the [Pulumi
  documentation](https://www.pulumi.com/docs/reference/yaml). [#261](https://github.com/pulumi/pulumi-yaml/pull/261)

### Bug Fixes

- Warn when using the reserved prefix `Fn::` as a map key.
  [#272](https://github.com/pulumi/pulumi-yaml/pull/272)

## 0.5.2 (2022-06-02)

### Improvements

- Fix `pulumi convert` panicking on programs containing `Fn::ToJSON`.
  [#250](https://github.com/pulumi/pulumi-yaml/pull/250)

- Fix `pulumi convert` panicking on programs containing a `Fn::Invoke` with an empty arguments
  property.
  [#262](https://github.com/pulumi/pulumi-yaml/pull/262)

- Fix `pulumi convert` panicking on programs containing `Fn::Secret`.
  [#260](https://github.com/pulumi/pulumi-yaml/pull/260)

### Bug Fixes

- Handle token types in the type checker.
  [#248](https://github.com/pulumi/pulumi-yaml/pull/248)

## 0.5.1 (2022-06-02)

### Improvements

- Update pulumi/pulumi to v3.32.1

- Add errors when hanging invalid fields off of resources.
  [#203](https://github.com/pulumi/pulumi-yaml/pull/203)

- Add errors when hanging invalid fields off of resource options.
  [#211](https://github.com/pulumi/pulumi-yaml/pull/211)

- Add a type checker.
  [#228](https://github.com/pulumi/pulumi-yaml/pull/228)

- Add `Fn::FromBase64`
  [#218](https://github.com/pulumi/pulumi-yaml/pull/218)

- Add support for Fn::ReadFile, enabling [Stack README](https://www.pulumi.com/blog/stack-readme/) support.
  [#217](https://github.com/pulumi/pulumi-yaml/pull/217)

- Allow Fn::Join to take expressions as inputs, previously the second argument had to be a syntactical list.
  [#241](https://github.com/pulumi/pulumi-yaml/pull/241)

### Bug Fixes

- De-duplicate error message added during pre-eval checking.
  [#207](https://github.com/pulumi/pulumi-yaml/pull/207)

- Prevent invokes without inputs from crashing `pulumi-language-yaml`.
  [#216](https://github.com/pulumi/pulumi-yaml/pull/216)

- Allow Fn::ToBase64 to take expressions as inputs, was previously constrained to only allow a
  string constant.
  [#221](https://github.com/pulumi/pulumi-yaml/pull/221)

- [expr] Fix handling of "plain" input maps when sending properties to component providers such as AWSX.
  [#195](https://github.com/pulumi/pulumi-yaml/pull/195)

- Do not panic when converting functions without `Return` fields.
  [#233](https://github.com/pulumi/pulumi-yaml/pull/234)

- Handle token types in the type checker.
  [#248](https://github.com/pulumi/pulumi-yaml/pull/248)

## 0.3.0 (2022-05-03)

### Improvements

- [language] integrated support for using another program or script to generate YAML

- [CLI] full `pulumi convert` support including support for "compilers"

- [codegen] Docs generation

## 0.2.0 (2022-04-26)

### Improvements

- Finalized expression language and documented built-in functions (`Fn::` items).

- Simplified `configuration` key

- Code generation improvements for conversion to other languages

### Bug Fixes

- Fixed rendering of known resource outputs during preview, secret outputs

- Error on invalid resource and invoke calls

- Support for Kubernetes resource "kind" and "apiVersion" constants

- Improved error messages

## 0.1.0 (2022-03-25)

First preview release of YAML language support for Pulumi. See README for language specification.

### Improvements

- [ci] Enable CI suite, testing.

- [features] Resources may specify "pluginDownloadURL" to exercise the common API signature used
  across providers for dynamic plugin acquisition.

- [features] Fn::Invoke arguments may contain outputs from other resources

- [features] Fn::Asset supports FileArchive, RemoteArchive modes

- [features] variables top level item implemented, can use variables to store intermediates to
  simplify invokes or re-use values

- [features] built-in variable "pulumi", which is a map with a "cwd", "stack", and "project" to
  obtain the current working directory, stack name, and project name respectively.

- [features] optional `Return` from `Fn::Invoke`

- [features] an expression referring to a resource by name, such as `${resource}` returns the
  resource object instead of an individual resource. Resource IDs are now obtained via `id` property
  in expressions. Example: use `${resource.id}` instead of `${resource}`, implements
  [#73](https://github.com/pulumi/pulumi-yaml/issues/73). Also adds a `urn` property to obtain the
  resource's URN.

- [features] no longer need to specify `component` property on resources, instead this is determined
  by discovering the package schema and using the value declared there.

- [features] can use shorter resource types and function names against one of a couple patterns.
