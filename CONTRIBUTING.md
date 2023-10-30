# Contributing

## Changelog

The changelog in this repo is managed through a `CHANGELOG_PENDING.md`
file.  Please add a new entry for any changes there.

## Releasing

To release a new version of `pulumi-yaml`, create a `CHANGELOG.md`
entry with the new version number, and copy the contents of
`CHANGELOG_PENDING.md` there.  Once this is merged, create a new tag
with the version number and push that.  This will kick off the
automation to create a new GitHub release.  Finally empty the
`CHANGELOG_PENDING.md` file, so we're ready for accumulating changelog
entries for the next version.

To release the version to users, `pulumi-yaml` also has to be updated
in `pulumi/pulumi`.
