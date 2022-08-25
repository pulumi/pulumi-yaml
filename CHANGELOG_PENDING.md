### Improvements

- Show all error/ warning messages
  [#279](https://github.com/pulumi/pulumi-yaml/pull/279)

- Support options on `Fn::Invoke`.

  [#284](https://github.com/pulumi/pulumi-yaml/pull/284)

- Add `Get` to resources, allowing Pulumi YAML programs to read external resources.
  [#290](https://github.com/pulumi/pulumi-yaml/pull/290)

- pulumi yaml syntax errors emit one less log message
  [#300](https://github.com/pulumi/pulumi-yaml/pull/300)

- Support setting default providers
  [#296](https://github.com/pulumi/pulumi-yaml/pull/296)

- Add sugar for `Fn::Invoke`
  [#294](https://github.com/pulumi/pulumi-yaml/pull/294)

- CI check to ensure `pu/pkg` and `pu/sdk` versions are merged
  [#307](https://github.com/pulumi/pulumi-yaml/pull/307)

- Add `Int` type to the configuration section.
  [#313](https://github.com/pulumi/pulumi-yaml/pull/313)

- Support `options.version` on `pulumi convert`
  [#291](https://github.com/pulumi/pulumi-yaml/pull/291)

### Bug Fixes

- Allow `bool` configuration type
  [#299](https://github.com/pulumi/pulumi-yaml/pull/299)

- Fix `pulumi convert` panic on `Fn::Split`
  [#319](https://github.com/pulumi/pulumi-yaml/pull/319)
