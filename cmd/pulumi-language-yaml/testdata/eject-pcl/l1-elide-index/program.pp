resource myStash "pulumi:index:Stash" {
	__logicalName = "myStash"
	input = "test"
}

output stashInput {
	__logicalName = "stashInput"
	value = myStash.input
}

output stashOutput {
	__logicalName = "stashOutput"
	value = myStash.output
}
