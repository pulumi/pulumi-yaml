resource res1 "simple:index:Resource" {
	__logicalName = "res1"
	value = true
}

output name {
	__logicalName = "name"
	value = pulumiResourceName(res1)
}

output type {
	__logicalName = "type"
	value = pulumiResourceType(res1)
}
