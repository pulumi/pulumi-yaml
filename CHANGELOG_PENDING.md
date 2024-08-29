### Improvements

- [features] add "pulumi.organiztion" to the built-in "pulumi" variable to obtain the current organization.

- [features] yaml runtime now uses the same plugin loader as the engine.

### Bug Fixes

- Parse the items property on config type declarations to prevent diagnostic messages about
  unknown fields [#615](https://github.com/pulumi/pulumi-yaml/pull/615)
