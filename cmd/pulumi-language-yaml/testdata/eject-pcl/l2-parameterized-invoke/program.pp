package {
	baseProviderName = "parameterized"
	baseProviderVersion = "1.2.3"

	parameterization {
		name = "subpackage"
		version = "2.0.0"
		value = "SGVsbG9Xb3JsZA=="
	}
}

// The invoke name is based on the parameter value
output parameterValue {
	__logicalName = "parameterValue"
	value = invoke("subpackage:index:doHelloWorld", {
		input = "goodbye"
	}).output
}
