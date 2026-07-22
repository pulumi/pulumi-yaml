package {
	baseProviderName = "extbase"
	baseProviderVersion = "45.0.0"

	parameterization {
		name = "myext"
		version = "2.0.0"
		value = "SGVsbG8="
	}
}

resource greeting "extbase:index:Greeting" {
	__logicalName = "greeting"
}

resource base "extbase:index:Base" {
	__logicalName = "base"
}

output parameterValue {
	__logicalName = "parameterValue"
	value = greeting.parameterValue
}

output baseValue {
	__logicalName = "baseValue"
	value = base.baseValue
}
