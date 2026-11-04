// A resource whose `elementType` property collides with the `ElementType()` method that
// generated Go SDK types must implement.
resource elem "reservednames:index:ElementType" {
	__logicalName = "elem"
	elementType = {
		elementType = "nested"
	}
}

// The whole object checks that languages serialize typed objects in stack outputs with
// their wire-format (camelCase) keys; the nested string exercises traversal through the
// renamed members.
output elementType {
	__logicalName = "elementType"
	value = elem.elementType
}

output nested {
	__logicalName = "nested"
	value = elem.elementType.elementType
}
