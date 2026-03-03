resource myRes "enum:index:Res" {
	__logicalName = "myRes"
	intEnum = 1
	stringEnum = "one"
}

resource mySink "extenumref:index:Sink" {
	__logicalName = "mySink"
	stringEnum = "two"
}
