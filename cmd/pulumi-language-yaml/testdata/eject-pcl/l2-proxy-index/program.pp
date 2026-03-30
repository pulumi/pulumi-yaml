resource res "ref-ref:index:Resource" {
	__logicalName = "res"
	data = {
		innerData = {
			boolean = false,
			float = 2.17,
			integer = -12,
			string = "Goodbye",
			boolArray = [
				false,
				true
			],
			stringMap = {
				"two" = "turtle doves",
				"three" = "french hens"
			}
		},
		boolean = true,
		float = 4.5,
		integer = 1024,
		string = "Hello",
		boolArray = [true],
		stringMap = {
			"x" = "100",
			"y" = "200"
		}
	}
}

output bool {
	__logicalName = "bool"
	value = res.data.boolean
}

output array {
	__logicalName = "array"
	value = res.data.boolArray[0]
}

output map {
	__logicalName = "map"
	value = res.data.stringMap.x
}

output nested {
	__logicalName = "nested"
	value = res.data.innerData.stringMap.three
}
