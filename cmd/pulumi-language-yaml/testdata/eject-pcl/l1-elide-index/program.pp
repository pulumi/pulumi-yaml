// Test that "pkg:typ" type tokens are accepted in PCL and are correctly expanded out. We also have an L2 test around
// this but it's worth checking with the pulumi schema as it would be too easy for codegen to special case it differently.
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
