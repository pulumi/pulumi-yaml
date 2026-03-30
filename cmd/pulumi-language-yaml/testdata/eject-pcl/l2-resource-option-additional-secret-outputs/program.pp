resource withSecret "simple:index:Resource" {
	__logicalName = "withSecret"
	value = true

	options {
		additionalSecretOutputs = [value]
	}
}

resource withoutSecret "simple:index:Resource" {
	__logicalName = "withoutSecret"
	value = true
}
