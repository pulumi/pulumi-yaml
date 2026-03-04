resource ref "pulumi:pulumi:StackReference" {
	__logicalName = "ref"
	name = "organization/other/dev"
}

output plain {
	__logicalName = "plain"
	value = ref.outputs.plain
}

output secret0 {
	__logicalName = "secret"
	value = ref.outputs.secret
}
