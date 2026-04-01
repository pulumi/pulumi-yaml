resource res "large:index:String" {
	__logicalName = "res"
	value = "hello world"
}

output output {
	__logicalName = "output"
	value = res.value
}
