resource res "large:index:Map" {
	__logicalName = "res"
	value = "leaf"
	depth = 300
}

output output {
	__logicalName = "output"
	value = res.value
}
