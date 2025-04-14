config inputs string {
	__logicalName = "inputs"
}

resource randomPassword "random:index/randomPassword:RandomPassword" {
	__logicalName = "randomPassword"
	keepers = inputs.keepers
	length = inputs.length
	lower = inputs.lower
	minLower = inputs.minLower
	minNumeric = inputs.minNumeric
	minSpecial = inputs.minSpecial
	minUpper = inputs.minUpper
	number = inputs.number
	numeric = inputs.numeric
	overrideSpecial = inputs.overrideSpecial
	special = inputs.special
	upper = inputs.upper
}

output password {
	__logicalName = "password"
	value = randomPassword.result
}
