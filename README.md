# PulumiFormation

A YAML and JSON language provider for Pulumi, inspired by CloudFormation.

> Note: this project includes a fork of the Pulumi Go SDK at commit 84d9947c6b9d67a0f86190a45698b10cde26db56.
> The SDK  assumes a stronger-typed view of the world than is reasonable when dealing with YAML/JSON untyped bags
> of properties. I suspect the changes will end up being minimal enough that we can upstream them afterwards.
