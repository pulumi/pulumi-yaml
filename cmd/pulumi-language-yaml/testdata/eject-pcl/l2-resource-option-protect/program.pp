resource protected "simple:index:Resource" {
	__logicalName = "protected"
	value = true

	options {
		protect = true
	}
}

resource unprotected "simple:index:Resource" {
	__logicalName = "unprotected"
	value = true

	options {
		protect = false
	}
}

resource defaulted "simple:index:Resource" {
	__logicalName = "defaulted"
	value = true
}
