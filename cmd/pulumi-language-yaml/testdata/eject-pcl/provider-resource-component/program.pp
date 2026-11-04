resource res "conformance-component:index:Simple" {
	__logicalName = "res"
	value = true
}

// Make a simple resource so that plugin detection works.
resource simpleResource "simple:index:Resource" {
	__logicalName = "simpleResource"
	value = false
}
