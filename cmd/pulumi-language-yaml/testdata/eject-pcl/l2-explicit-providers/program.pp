resource explicit "pulumi:providers:component" {
	__logicalName = "explicit"
}

resource list "component:index:ComponentCallable" {
	__logicalName = "list"
	value = "value"

	options {
		providers = [explicit]
	}
}

resource map "component:index:ComponentCallable" {
	__logicalName = "map"
	value = "value"

	options {
		providers = [explicit]
	}
}
