### Improvements

- [ci] Enable CI suite, testing.

- [feature] Resources may specify "pluginDownloadURL" to exercise the common API signature used
  across providers for dynamic plugin acquisition.

- [features] Fn::Invoke arguments may contain outputs from other resources

- [features] Fn::Asset supports FileArchive, RemoteArchive modes

- [features] variables top level item implemented, can use variables to store intermediates to
  simplify invokes or re-use values

- [features] built-in variable "pulumi", which is a map with a "cwd", "stack", and "project" to
  obtain the current working directory, stack name, and project name respectively.

- [features] optional `Return` from `Fn::Invoke`

### Bug Fixes
