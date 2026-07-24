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

resource greetingComp "extbase:index:GreetingComponent" {
	__logicalName = "greetingComp"
}

output parameterValue {
	__logicalName = "parameterValue"
	value = greeting.parameterValue
}

output parameterValueFromComponent {
	__logicalName = "parameterValueFromComponent"
	value = greetingComp.parameterValue
}

output invokeGreeting {
	__logicalName = "invokeGreeting"
	value = invoke("extbase:index:greet", {
		name = "Pulumi"
	}).greeting
}
