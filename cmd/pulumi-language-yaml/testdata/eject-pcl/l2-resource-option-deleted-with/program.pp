resource target "simple:index:Resource" {
	__logicalName = "target"
	value = true
}

resource deletedWith "simple:index:Resource" {
	__logicalName = "deletedWith"
	value = true

	options {
		deletedWith = target
	}
}

resource notDeletedWith "simple:index:Resource" {
	__logicalName = "notDeletedWith"
	value = true
}
