config input string {
	__logicalName = "input"
}

hashVar = sha1(input)

output hash {
	__logicalName = "hash"
	value = hashVar
}
