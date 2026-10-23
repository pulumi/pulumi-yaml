resource failing "fail_on_create:index:Resource" {
	__logicalName = "failing"
	value = false
}

resource dependent "simple:index:Resource" {
	__logicalName = "dependent"
	value = failing.value
}

resource independent "simple:index:Resource" {
	__logicalName = "independent"
	value = true
}

output failingValue {
	__logicalName = "failingValue"
	value = failing.value
}
