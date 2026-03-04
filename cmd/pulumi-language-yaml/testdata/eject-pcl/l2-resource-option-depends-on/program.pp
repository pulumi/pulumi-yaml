resource noDependsOn "simple:index:Resource" {
	__logicalName = "noDependsOn"
	value = true
}

resource withDependsOn "simple:index:Resource" {
	__logicalName = "withDependsOn"
	value = false

	options {
		dependsOn = [noDependsOn]
	}
}
