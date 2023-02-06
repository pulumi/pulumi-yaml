### Improvements

- Deprecate `fn::stackReference`.
  [#420](https://github.com/pulumi/pulumi-yaml/pull/420)

- Ensure resource and invoke option "version" is used in package resolution, enabling Docker v4
  provider `docker:Image` resource.
  [#423](https://github.com/pulumi/pulumi-yaml/pull/423)

- Introduce `fn::method` function for resource methods.
  [#431](https://github.com/pulumi/pulumi-yaml/pull/431)

- Add support for the `deletedWith` resource option.
  [#437](https://github.com/pulumi/pulumi-yaml/pull/437)

### Bug Fixes

- Avoid panicing for non-string map keys
  [#428](https://github.com/pulumi/pulumi-yaml/pull/428)
