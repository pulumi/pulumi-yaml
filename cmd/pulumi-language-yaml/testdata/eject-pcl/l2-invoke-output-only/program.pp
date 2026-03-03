output hello {
	__logicalName = "hello"
	value = invoke("output-only-invoke:index:myInvoke", {
		value = "hello"
	}).result
}

output goodbye {
	__logicalName = "goodbye"
	value = invoke("output-only-invoke:index:myInvoke", {
		value = "goodbye"
	}).result
}
