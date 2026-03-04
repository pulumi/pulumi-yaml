resource targetOnly "simple:index:Resource" {
	__logicalName = "targetOnly"
	value = true
}

resource unrelated "simple:index:Resource" {
	__logicalName = "unrelated"
	value = true

	options {
		dependsOn = [dep]
	}
}

resource dep "simple:index:Resource" {
	__logicalName = "dep"
	value = true
}
