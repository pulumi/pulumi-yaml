resource withV2 "simple:index:Resource" {
	__logicalName = "withV2"
	value = true

	options {
		version = "2.0.0"
	}
}

resource withV26 "simple:index:Resource" {
	__logicalName = "withV26"
	value = false

	options {
		version = "26.0.0"
	}
}

resource withDefault "simple:index:Resource" {
	__logicalName = "withDefault"
	value = true

	options {
		version = "26.0.0"
	}
}
