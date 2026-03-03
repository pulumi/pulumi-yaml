resource ignoreChanges "simple:index:Resource" {
	__logicalName = "ignoreChanges"
	value = true

	options {
		ignoreChanges = [value]
	}
}

resource notIgnoreChanges "simple:index:Resource" {
	__logicalName = "notIgnoreChanges"
	value = true
}
