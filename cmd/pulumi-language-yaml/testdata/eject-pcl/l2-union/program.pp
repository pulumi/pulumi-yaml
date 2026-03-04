resource stringOrIntegerExample1 "union:index:Example" {
	__logicalName = "stringOrIntegerExample1"
	stringOrIntegerProperty = 42
}

resource stringOrIntegerExample2 "union:index:Example" {
	__logicalName = "stringOrIntegerExample2"
	stringOrIntegerProperty = "forty two"
}

resource mapMapUnionExample "union:index:Example" {
	__logicalName = "mapMapUnionExample"
	mapMapUnionProperty = {
		"key1" = {
			"key1a" = "value1a"
		}
	}
}

resource stringEnumUnionListExample "union:index:Example" {
	__logicalName = "stringEnumUnionListExample"
	stringEnumUnionListProperty = [
		"Listen",
		"Send",
		"NotAnEnumValue"
	]
}

resource safeEnumExample "union:index:Example" {
	__logicalName = "safeEnumExample"
	typedEnumProperty = "Block"
}

resource enumOutputExample "union:index:EnumOutput" {
	__logicalName = "enumOutputExample"
	name = "example"
}

resource outputEnumExample "union:index:Example" {
	__logicalName = "outputEnumExample"
	typedEnumProperty = enumOutputExample.type
}

output mapMapUnionOutput {
	__logicalName = "mapMapUnionOutput"
	value = mapMapUnionExample.mapMapUnionProperty
}
