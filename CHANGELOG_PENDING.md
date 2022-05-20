### Improvements

- Update pulumi/pulumi to v3.32.1

- Add errors when hanging invalid fields off of resources.  
  [#203](https://github.com/pulumi/pulumi-yaml/pull/203)

- Add errors when hanging invalid fields off of resource options.  
  [#211](https://github.com/pulumi/pulumi-yaml/pull/211)

### Bug Fixes

- De-duplicate error message added during pre-eval checking.  
  [#207](https://github.com/pulumi/pulumi-yaml/pull/207)

- Prevent invokes without inputs from crashing `pulumi-language-yaml`.  
  [#216](https://github.com/pulumi/pulumi-yaml/pull/216)
