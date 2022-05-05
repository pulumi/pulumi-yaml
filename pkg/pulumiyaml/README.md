# pulumiyaml Implementation Guide

The pulumiyaml package handles the core logic of Pulumi YAML.

## Token Resolution

### Algorithm

Given a schema token `T` (for either a resource or function), Pulumi YAML
attempts to resolve it to a schema defined resource/function with the following
algorithm:

1. If `T` is prefixed with `pulumi:provider:` and `T` is a resource token,
   resolve it as a provider and return.
2. If `T` is mentioned explicitly in the schema, either as a resource name or an
   alias, return the named resource.
3. If `T` is of the form `${package}:${resource}`, set `T` to
   `${package}:index:${resource}`. Goto 2.
4. If `T` is of the form `${package}:${mod}:${resource}`, set `T` to
   `${package}:${mod}/${camelCase(resource)}:${resource}`. Goto 2.
5. Return no resource found.

where `${...}` matches any characters except for `:` and `/`.

### Examples

- `foo:Bar` will resolve to itself, then `foo:index:Bar`, then `foo:index/bar:Bar`.
- `foo:mod:Bar` will resolve to itself, then `foo:mod/bar:Bar`.
- `foo:mod/bar:Bar` will resolve only to itself.
- `pulumi:provider:Foo` will resolve only to itself.
