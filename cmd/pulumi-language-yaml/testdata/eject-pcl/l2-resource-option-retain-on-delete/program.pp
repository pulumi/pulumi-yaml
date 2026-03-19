resource retainOnDelete "simple:index:Resource" {
	__logicalName = "retainOnDelete"
	value = true

	options {
		retainOnDelete = true
	}
}

resource notRetainOnDelete "simple:index:Resource" {
	__logicalName = "notRetainOnDelete"
	value = true

	options {
		retainOnDelete = false
	}
}

resource defaulted "simple:index:Resource" {
	__logicalName = "defaulted"
	value = true
}
