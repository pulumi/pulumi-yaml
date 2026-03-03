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
