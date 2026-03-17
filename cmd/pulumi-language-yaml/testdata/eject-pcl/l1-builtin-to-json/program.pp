config aString string {
	__logicalName = "aString"
}

config aNumber number {
	__logicalName = "aNumber"
}

nestedObject = {
	"anObject" = {
		"name" = aString,
		"items" = aList
	},
	"a_secret" = aSecret
}

output stringOutput {
	__logicalName = "stringOutput"
	value = toJSON(aString)
}

output numberOutput {
	__logicalName = "numberOutput"
	value = toJSON(aNumber)
}

output boolOutput {
	__logicalName = "boolOutput"
	value = toJSON(true)
}

output arrayOutput {
	__logicalName = "arrayOutput"
	value = toJSON([
		"x",
		"y",
		"z"
	])
}

output objectOutput {
	__logicalName = "objectOutput"
	value = toJSON({
		"key" = "value",
		"count" = 1
	})
}

output nestedOutput {
	__logicalName = "nestedOutput"
	value = toJSON(nestedObject)
}
