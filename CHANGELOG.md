CHANGELOG
=========

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

* [language] integrated support for using another program or script to generate YAML

* [CLI] full `pulumi convert` support including support for "compilers"

* [codegen] Docs generation

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
