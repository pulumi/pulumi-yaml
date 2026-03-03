resource first "simple:index:Resource" {
	__logicalName = "first"
	value = false
}

resource second "simple:index:Resource" {
	__logicalName = "second"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = "hello",
		secretResponse = first.value
	}).secret
}
