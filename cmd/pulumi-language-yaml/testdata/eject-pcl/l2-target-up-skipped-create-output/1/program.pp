resource target "simple:index:Resource" {
	__logicalName = "target"
	value = true
}

resource other "nestedobject:index:Container" {
	__logicalName = "other"
	inputs = ["a"]
}

resource skipped "nestedobject:index:Container" {
	__logicalName = "skipped"
	inputs = ["b"]
}

output skippedOutput {
	__logicalName = "skippedOutput"
	value = "skipped-${skipped.details[0].key}"
}
