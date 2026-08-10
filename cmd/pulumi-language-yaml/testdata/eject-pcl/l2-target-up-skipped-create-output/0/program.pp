resource target "simple:index:Resource" {
	__logicalName = "target"
	value = true
}

resource other "nestedobject:index:Container" {
	__logicalName = "other"
	inputs = ["a"]
}
