localVar = res1.value

resource res2 "simple:index:Resource" {
	__logicalName = "res2"
	value = localVar
}

resource res1 "simple:index:Resource" {
	__logicalName = "res1"
	value = true
}

output out {
	__logicalName = "out"
	value = res2.value
}
