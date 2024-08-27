### Improvements

- [features] add "pulumi.organiztion" to the built-in "pulumi" variable to obtain the current organization.

### Bug Fixes

- Parse the items property on config type declarations to prevent diagnostic messages about
  unknown fields [#615](https://github.com/pulumi/pulumi-yaml/pull/615)

- Allow missing nodes in template to enable walking templates without config
  [#617](https://github.com/pulumi/pulumi-yaml/pull/617)
