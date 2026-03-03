localValue = "hello"

output dynamic {
	__logicalName = "dynamic"
	value = invoke("any-type-function:index:dynListToDyn", {
		inputs = [
			"hello",
			localValue,
			{}
		]
	}).result
}
