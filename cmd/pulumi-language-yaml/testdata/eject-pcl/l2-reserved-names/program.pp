resource elem "reservednames:index:ElementType" {
	__logicalName = "elem"
	elementType = {
		elementType = "nested"
	}
}

output elementType {
	__logicalName = "elementType"
	value = elem.elementType.elementType
}
