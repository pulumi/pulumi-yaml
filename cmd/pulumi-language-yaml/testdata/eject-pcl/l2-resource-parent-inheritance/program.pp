resource provider "pulumi:providers:simple" {
	__logicalName = "provider"
}

resource parent1 "simple:index:Resource" {
	__logicalName = "parent1"
	value = true

	options {
		provider = provider
	}
}

resource child1 "simple:index:Resource" {
	__logicalName = "child1"
	value = true

	options {
		parent = parent1
	}
}

resource orphan1 "simple:index:Resource" {
	__logicalName = "orphan1"
	value = true
}

resource parent2 "simple:index:Resource" {
	__logicalName = "parent2"
	value = true

	options {
		protect = true
		retainOnDelete = true
	}
}

resource child2 "simple:index:Resource" {
	__logicalName = "child2"
	value = true

	options {
		parent = parent2
	}
}

resource child3 "simple:index:Resource" {
	__logicalName = "child3"
	value = true

	options {
		parent = parent2
		protect = false
		retainOnDelete = false
	}
}

resource orphan2 "simple:index:Resource" {
	__logicalName = "orphan2"
	value = true
}
