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
