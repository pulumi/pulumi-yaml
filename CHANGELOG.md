CHANGELOG
=========

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
