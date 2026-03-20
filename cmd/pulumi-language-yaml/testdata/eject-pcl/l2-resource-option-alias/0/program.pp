resource parent "simple:index:Resource" {
	__logicalName = "parent"
	value = true
}

resource aliasURN "simple:index:Resource" {
	__logicalName = "aliasURN"
	value = true
}

resource aliasName "simple:index:Resource" {
	__logicalName = "aliasName"
	value = true
}

resource aliasNoParent "simple:index:Resource" {
	__logicalName = "aliasNoParent"
	value = true
}

resource aliasParent "simple:index:Resource" {
	__logicalName = "aliasParent"
	value = true

	options {
		parent = aliasURN
	}
}

resource aliasType "component:index:Custom" {
	__logicalName = "aliasType"
	value = "true"
}
