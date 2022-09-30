### Improvements

### Bug Fixes

- Fix generated `Pulumi.yaml` on convert
  [#339](https://github.com/pulumi/pulumi-yaml/pull/339)

- Enforce top level required properties
  [#350](https://github.com/pulumi/pulumi-yaml/pull/350)

- Don't panic when the typechecker rejects
  [#346](https://github.com/pulumi/pulumi-yaml/pull/346)

- Allow interpolations for `AssetOrArchive` function values
  [#341](https://github.com/pulumi/pulumi-yaml/pull/341)

- Clarify the lifetimes when calling `codegen.Eject`. This is a breaking change to the
  `codegen.Eject` API.
  [#358](https://github.com/pulumi/pulumi-yaml/pull/358)
