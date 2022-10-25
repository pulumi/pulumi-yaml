### Improvements

- Ensure that constant values passed to enums are valid.
  [#357](https://github.com/pulumi/pulumi-yaml/pull/357)

- Warn on non camelCase names.
  [#362](https://github.com/pulumi/pulumi-yaml/pull/362)

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
