resource target "simple:index:Resource" {
	__logicalName = "target"
	value = true
}

resource replaceWith "simple:index:Resource" {
	__logicalName = "replaceWith"
	value = true

	options {
		replaceWith = [target]
	}
}

resource notReplaceWith "simple:index:Resource" {
	__logicalName = "notReplaceWith"
	value = true
}
