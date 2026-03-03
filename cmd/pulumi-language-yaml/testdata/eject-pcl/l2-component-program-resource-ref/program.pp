resource component1 "component:index:ComponentCustomRefOutput" {
	__logicalName = "component1"
	value = "foo-bar-baz"
}

resource custom1 "component:index:Custom" {
	__logicalName = "custom1"
	value = component1.value
}

resource custom2 "component:index:Custom" {
	__logicalName = "custom2"
	value = component1.ref.value
}
