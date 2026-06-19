resource enumRes "enum:index:Res" {
	__logicalName = "enumRes"
	intEnum = 1
	stringEnum = "one"
}

resource res "docs:index:Resource" {
	__logicalName = "res"
	in = invoke("docs:index:fun", {
		in = false
	}).out
	externalEnum = "one"
}
