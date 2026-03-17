resource firstResource "keywords:index:SomeResource" {
	__logicalName = "firstResource"
	builtins = "builtins"
	lambda = "lambda"
	property = "property"
}

resource secondResource "keywords:index:SomeResource" {
	__logicalName = "secondResource"
	builtins = firstResource.builtins
	lambda = firstResource.lambda
	property = firstResource.property
}

resource lambdaModuleResource "keywords:lambda:SomeResource" {
	__logicalName = "lambdaModuleResource"
	builtins = "builtins"
	lambda = "lambda"
	property = "property"
}

resource lambdaResource "keywords:index:Lambda" {
	__logicalName = "lambdaResource"
	builtins = "builtins"
	lambda = "lambda"
	property = "property"
}
