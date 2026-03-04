output hello {
	__logicalName = "hello"
	value = invoke("simple-invoke:index:myInvoke", {
		value = "hello"
	}).result
}

output goodbye {
	__logicalName = "goodbye"
	value = invoke("simple-invoke:index:myInvoke", {
		value = "goodbye"
	}).result
}
