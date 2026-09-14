resource r "output:index:Resource" {
	__logicalName = "r"
	value = 1
}

output wrapped {
	__logicalName = "wrapped"
	value = secret(r.output)
}
