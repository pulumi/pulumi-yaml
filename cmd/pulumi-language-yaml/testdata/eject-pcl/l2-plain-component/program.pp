resource myComponent "plaincomponent:index:Component" {
	__logicalName = "myComponent"
	name = "my-resource"
	settings = {
		enabled = true,
		tags = {
			"env" = "test"
		}
	}
}

output label {
	__logicalName = "label"
	value = myComponent.label
}
