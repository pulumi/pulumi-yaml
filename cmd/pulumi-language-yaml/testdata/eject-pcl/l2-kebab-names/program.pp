resource first "kebab-names:kebab-module:someResource" {
	__logicalName = "first"
	theInput = true
	nested = {
		nestedValue = "nested"
	}
}

resource second "kebab-names:kebab-module:anotherResource" {
	__logicalName = "second"
	theInput = first.theOutput.nestedOutput
}
