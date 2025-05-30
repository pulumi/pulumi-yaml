# Contributing

## Getting started

The tools for this repository can be installed using [`mise`][mise]. Once your
shell has been configured to [activate `mise`][activate-mise], navigate to the
repository, run `mise trust` to allow the installation of the tools listed in
`.mise.toml`, and then `mise install` to install anything missing.

## Changelog

Changelog management is done via [`changie`](https://changie.dev/).
See the [installation](https://changie.dev/guide/installation/) guide for `changie`.

Run `changie new` in the top level directory. Here is an example of what that looks like:

```shell
$ changie new
✔ Component … runtime
✔ Kind … Improvements
✔ Body … Cool new feature.
✔ GitHub Pull Request … 123
```

## Release

To release a new version use `changie` to update the changelog file, open a PR for that change. Once that PR merges it will trigger a release workflow.

```shell
$ changie batch auto
$ changie merge
$ git add .
$ git commit -m "Changelog for $(changie latest)"
```

After the release, also bump the version in `pulumi/pulumi`.  Do this by updating the version in the [get-language-providers.sh script](https://github.com/pulumi/pulumi/blob/master/scripts/get-language-providers.sh#L35) in the pulumi/pulumi repository.

[mise]: https://mise.jdx.dev/getting-started.html#installing-mise-cli
[activate-mise]: https://mise.jdx.dev/getting-started.html#activate-mise
