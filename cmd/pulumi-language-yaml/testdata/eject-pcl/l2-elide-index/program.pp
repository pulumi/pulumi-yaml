resource res "simple:index:Resource" {
	__logicalName = "res"
	value = true
}

output inv {
	__logicalName = "inv"
	value = invoke("simple-invoke:index:myInvoke", {
		value = "test"
	}).result
}
