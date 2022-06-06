### Improvements

- Update pulumi/pulumi to v3.32.1

- Add errors when hanging invalid fields off of resources.
  [#203](https://github.com/pulumi/pulumi-yaml/pull/203)

- Add errors when hanging invalid fields off of resource options.
  [#211](https://github.com/pulumi/pulumi-yaml/pull/211)

- Add a type checker.
  [#228](https://github.com/pulumi/pulumi-yaml/pull/228)

- Add `Fn::FromBase64`
  [#218](https://github.com/pulumi/pulumi-yaml/pull/218)

- Add support for Fn::ReadFile, enabling [Stack README](https://www.pulumi.com/blog/stack-readme/) support.
  [#217](https://github.com/pulumi/pulumi-yaml/pull/217)

- Allow Fn::Join to take expressions as inputs, previously the second argument had to be a syntactical list.
  [#241](https://github.com/pulumi/pulumi-yaml/pull/241)

### Bug Fixes

- De-duplicate error message added during pre-eval checking.
  [#207](https://github.com/pulumi/pulumi-yaml/pull/207)

- Prevent invokes without inputs from crashing `pulumi-language-yaml`.
  [#216](https://github.com/pulumi/pulumi-yaml/pull/216)

- Allow Fn::ToBase64 to take expressions as inputs, was previously constrained to only allow a
  string constant.
  [#221](https://github.com/pulumi/pulumi-yaml/pull/221)

- [expr] Fix handling of "plain" input maps when sending properties to component providers such as AWSX.
  [#195](https://github.com/pulumi/pulumi-yaml/pull/195)

- Do not panic when converting functions without `Return` fields.
  [#233](https://github.com/pulumi/pulumi-yaml/pull/234)
