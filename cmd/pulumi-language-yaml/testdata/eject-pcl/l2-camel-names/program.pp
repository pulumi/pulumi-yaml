resource firstResource "camelNames:CoolModule:SomeResource" {
	__logicalName = "firstResource"
	theInput = true
}

resource secondResource "camelNames:CoolModule:SomeResource" {
	__logicalName = "secondResource"
	theInput = firstResource.theOutput
}

resource thirdResource "camelNames:CoolModule:SomeResource" {
	__logicalName = "thirdResource"
	theInput = true
	resourceName = "my-cluster"
}
