output secret0 {
	__logicalName = "secret"
	value = invoke("scalar-returns:index:invokeSecret", {
		value = "goodbye"
	})
}

output array {
	__logicalName = "array"
	value = invoke("scalar-returns:index:invokeArray", {
		value = "the word"
	})
}

output map {
	__logicalName = "map"
	value = invoke("scalar-returns:index:invokeMap", {
		value = "hello"
	})
}

output secretMap {
	__logicalName = "secretMap"
	value = invoke("scalar-returns:index:invokeMap", {
		value = "secret"
	})
}
