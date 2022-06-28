### Improvements

- Fix `pulumi convert` panicking on programs containing `Fn::ToJSON`.
  [#250](https://github.com/pulumi/pulumi-yaml/pull/250)

- Fix `pulumi convert` panicking on programs containing a `Fn::Invoke` with an empty arguments
  property.
  [#264](https://github.com/pulumi/pulumi-yaml/pull/264)

### Bug Fixes

- Handle token types in the type checker.
  [#248](https://github.com/pulumi/pulumi-yaml/pull/248)
