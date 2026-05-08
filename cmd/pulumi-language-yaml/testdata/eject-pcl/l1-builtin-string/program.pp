config aString string {
	__logicalName = "aString"
}

output lengthResult {
	__logicalName = "lengthResult"
	value = length(aString)
}

output splitResult {
	__logicalName = "splitResult"
	value = split("-", aString)
}

output joinResult {
	__logicalName = "joinResult"
	value = join("|", split("-", aString))
}

output interpolateResult {
	__logicalName = "interpolateResult"
	value = "prefix-${aString}"
}
