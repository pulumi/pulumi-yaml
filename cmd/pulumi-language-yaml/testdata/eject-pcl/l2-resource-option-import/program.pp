resource import "simple:index:Resource" {
	__logicalName = "import"
	value = true

	options {
		import = "fakeID123"
	}
}

resource notImport "simple:index:Resource" {
	__logicalName = "notImport"
	value = true
}
