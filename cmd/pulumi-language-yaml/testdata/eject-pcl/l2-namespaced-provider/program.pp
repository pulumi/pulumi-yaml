resource componentRes "component:index:ComponentCustomRefOutput" {
	__logicalName = "componentRes"
	value = "foo-bar-baz"
}

resource res "namespaced:index:Resource" {
	__logicalName = "res"
	value = true
	resourceRef = componentRes.ref
}
