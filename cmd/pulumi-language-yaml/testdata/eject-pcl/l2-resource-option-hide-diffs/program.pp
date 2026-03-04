resource hideDiffs "simple:index:Resource" {
	__logicalName = "hideDiffs"
	value = true

	options {
		hideDiffs = [value]
	}
}

resource notHideDiffs "simple:index:Resource" {
	__logicalName = "notHideDiffs"
	value = true
}
