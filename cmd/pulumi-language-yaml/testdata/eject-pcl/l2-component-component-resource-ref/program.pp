resource component1 "component:index:ComponentCustomRefOutput" {
	__logicalName = "component1"
	value = "foo-bar-baz"
}

resource component2 "component:index:ComponentCustomRefInputOutput" {
	__logicalName = "component2"
	inputRef = component1.ref
}

resource custom1 "component:index:Custom" {
	__logicalName = "custom1"
	value = component2.inputRef.value
}

resource custom2 "component:index:Custom" {
	__logicalName = "custom2"
	value = component2.outputRef.value
}
