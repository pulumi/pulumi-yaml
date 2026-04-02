resource replacementTrigger "simple:index:Resource" {
	__logicalName = "replacementTrigger"
	value = true

	options {
		replacementTrigger = "test"
	}
}

resource unknown "output:index:Resource" {
	__logicalName = "unknown"
	value = 1
}

resource unknownReplacementTrigger "simple:index:Resource" {
	__logicalName = "unknownReplacementTrigger"
	value = true

	options {
		replacementTrigger = "hellohello"
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
			1,
			2,
			3
		])
	}
}
