config input string {
	__logicalName = "input"
}

bytes = fromBase64(input)

output data {
	__logicalName = "data"
	value = bytes
}

output roundtrip {
	__logicalName = "roundtrip"
	value = toBase64(bytes)
}
