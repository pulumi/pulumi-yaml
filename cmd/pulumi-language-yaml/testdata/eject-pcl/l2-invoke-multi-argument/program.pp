output both {
	__logicalName = "both"
	value = invoke("multi-argument-invoke:index:multiArgumentInvoke", {
		first = "hello",
		second = "world"
	}).result
}

output onlyRequired {
	__logicalName = "onlyRequired"
	value = invoke("multi-argument-invoke:index:multiArgumentInvoke", {
		first = "hello"
	}).result
}
