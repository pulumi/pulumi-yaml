# Contributing

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

After the release, also bump the version in `pulumi/pulumi`.  Do this by updating the version of pulumi-yaml in the pkg/go.mod.