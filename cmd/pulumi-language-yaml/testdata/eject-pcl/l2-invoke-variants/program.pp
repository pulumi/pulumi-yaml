resource res "simple-invoke:index:StringResource" {
	__logicalName = "res"
	text = "hello"
}

output outputInput {
	__logicalName = "outputInput"
	value = invoke("simple-invoke:index:myInvoke", {
		value = res.text
	}).result
}

output unit {
	__logicalName = "unit"
	value = invoke("simple-invoke:index:unit", {}).result
}
