resource replacementTrigger "simple:index:Resource" {
	__logicalName = "replacementTrigger"
	value = true

	options {
		replacementTrigger = "test2"
	}
}

resource unknown "output:index:Resource" {
	__logicalName = "unknown"
	value = 2
}

resource unknownReplacementTrigger "simple:index:Resource" {
	__logicalName = "unknownReplacementTrigger"
	value = true

	options {
		replacementTrigger = unknown.output
	}
}

resource notReplacementTrigger "simple:index:Resource" {
	__logicalName = "notReplacementTrigger"
	value = true
}

resource secretReplacementTrigger "simple:index:Resource" {
	__logicalName = "secretReplacementTrigger"
	value = true

	options {
		replacementTrigger = secret([
			3,
			2,
			1
		])
	}
}
