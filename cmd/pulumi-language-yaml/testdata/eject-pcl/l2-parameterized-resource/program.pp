package {
	baseProviderName = "parameterized"
	baseProviderVersion = "1.2.3"

	parameterization {
		name = "subpackage"
		version = "2.0.0"
		value = "SGVsbG9Xb3JsZA=="
	}
}

resource example "subpackage:index:HelloWorld" {
	__logicalName = "example"
}

resource exampleComponent "subpackage:index:HelloWorldComponent" {
	__logicalName = "exampleComponent"
}

output parameterValue {
	__logicalName = "parameterValue"
	value = example.parameterValue
}

output parameterValueFromComponent {
	__logicalName = "parameterValueFromComponent"
	value = exampleComponent.parameterValue
}
