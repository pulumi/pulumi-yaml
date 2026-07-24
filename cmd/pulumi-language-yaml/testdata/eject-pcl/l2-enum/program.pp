resource sink1 "enum:index:Res" {
	__logicalName = "sink1"
	intEnum = 1
	stringEnum = "two"
}

resource sink2 "enum:mod:Res" {
	__logicalName = "sink2"
	intEnum = 1
	stringEnum = "two"
}

resource sink3 "enum:mod/nested:Res" {
	__logicalName = "sink3"
	intEnum = 1
	stringEnum = "two"
}

resource sink4 "enum:index:Deluxe" {
	__logicalName = "sink4"
	numberEnum = 0.1
	wordyEnum = "It's got apostrophes"
	arrayOfEnum = [
		"one",
		"two"
	]
	mapOfEnum = {
		"small" = 1,
		"large" = 2
	}
	holder = {
		size = 2,
		color = "one"
	}
	unionEnum = "A Value With Spaces."
}
