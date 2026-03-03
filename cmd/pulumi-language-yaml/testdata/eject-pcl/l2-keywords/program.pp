resource firstResource "keywords:index:SomeResource" {
	__logicalName = "firstResource"
	builtins = "builtins"
	property = "property"
}

resource secondResource "keywords:index:SomeResource" {
	__logicalName = "secondResource"
	builtins = firstResource.builtins
	property = firstResource.property
}
