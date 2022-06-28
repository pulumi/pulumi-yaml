### Improvements

- Fix `pulumi convert` panicking on programs containing `Fn::ToJSON`.
  [#250](https://github.com/pulumi/pulumi-yaml/pull/250)

- Fix `pulumi convert` panicking on programs containing `Fn::Secret`.
  [#260](https://github.com/pulumi/pulumi-yaml/pull/260)

- Fix `pulumi convert` panicking on programs containing a `Fn::Invoke` with an empty arguments
  property.
  [#262](https://github.com/pulumi/pulumi-yaml/pull/262)

### Bug Fixes

- Handle token types in the type checker.
  [#248](https://github.com/pulumi/pulumi-yaml/pull/248)
