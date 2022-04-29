### Improvements

- Finalized expression language and documented built-in functions (`Fn::` items).

- Simplified `configuration` key

- Code generation improvements for conversion to other languages

- Enable `runtime.options.compiler` to use an external tool to generate YAML or JSON, e.g.: Cue.

### Bug Fixes

- Fixed rendering of known resource outputs during preview, secret outputs

- Error on invalid resource and invoke calls

- Support for Kubernetes resource "kind" and "apiVersion" constants

- Improved error messages
