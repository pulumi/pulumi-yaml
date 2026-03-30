resource failing "fail_on_create:index:Resource" {
	__logicalName = "failing"
	value = false
}

resource dependent "simple:index:Resource" {
	__logicalName = "dependent"
	value = true

	options {
		dependsOn = [failing]
	}
}

resource dependentOnOutput "simple:index:Resource" {
	__logicalName = "dependent_on_output"
	value = failing.value
}

resource independent "simple:index:Resource" {
	__logicalName = "independent"
	value = true
}

resource doubleDependency "simple:index:Resource" {
	__logicalName = "double_dependency"
	value = true

	options {
		dependsOn = [
			independent,
			dependentOnOutput
		]
	}
}
