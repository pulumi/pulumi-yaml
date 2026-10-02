package {
	baseProviderName = "extbase"
	baseProviderVersion = "45.0.0"

	parameterization {
		name = "myext"
		version = "2.0.0"
		value = "SGVsbG8="
	}
}

resource prov "pulumi:providers:extbase" {
	__logicalName = "prov"
}

resource greeting "myext:index:Greeting" {
	__logicalName = "greeting"

	options {
		provider = prov
	}
}

resource base "extbase:index:Base" {
	__logicalName = "base"

	options {
		provider = prov
	}
}

output parameterValue {
	__logicalName = "parameterValue"
	value = greeting.parameterValue
}

output baseValue {
	__logicalName = "baseValue"
	value = base.baseValue
}
