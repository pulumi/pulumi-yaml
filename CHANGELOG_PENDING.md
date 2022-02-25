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

### Bug Fixes
