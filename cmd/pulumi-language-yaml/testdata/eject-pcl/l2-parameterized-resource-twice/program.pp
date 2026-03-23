package {
	baseProviderName = "parameterized"
	baseProviderVersion = "1.2.3"

	parameterization {
		name = "byepackage"
		version = "2.0.0"
		value = "R29vZGJ5ZVdvcmxk"
	}
}

package {
	baseProviderName = "parameterized"
	baseProviderVersion = "1.2.3"

	parameterization {
		name = "hipackage"
		version = "2.0.0"
		value = "SGVsbG9Xb3JsZA=="
	}
}

resource example1 "hipackage:index:HelloWorld" {
	__logicalName = "example1"
}

resource exampleComponent1 "hipackage:index:HelloWorldComponent" {
	__logicalName = "exampleComponent1"
}

resource example2 "byepackage:index:GoodbyeWorld" {
	__logicalName = "example2"
}

resource exampleComponent2 "byepackage:index:GoodbyeWorldComponent" {
	__logicalName = "exampleComponent2"
}

output parameterValue1 {
	__logicalName = "parameterValue1"
	value = example1.parameterValue
}

output parameterValueFromComponent1 {
	__logicalName = "parameterValueFromComponent1"
	value = exampleComponent1.parameterValue
}

output parameterValue2 {
	__logicalName = "parameterValue2"
	value = example2.parameterValue
}

output parameterValueFromComponent2 {
	__logicalName = "parameterValueFromComponent2"
	value = exampleComponent2.parameterValue
}
