package {
	baseProviderName = "parameterized"
	baseProviderVersion = "1.2.3"

	parameterization {
		name = "goodbye"
		version = "2.0.0"
		value = "R29vZGJ5ZQ=="
	}
}

resource prov "pulumi:providers:goodbye" {
	__logicalName = "prov"
	text = "World"
}

// The resource name is based on the parameter value
resource res "goodbye:index:Goodbye" {
	__logicalName = "res"

	options {
		provider = prov
	}
}

// The resource name is based on the parameter value and the provider config
output parameterValue {
	__logicalName = "parameterValue"
	value = res.parameterValue
}
