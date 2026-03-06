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

resource parent2 "primitive:index:Resource" {
	__logicalName = "parent2"
	boolean = false
	float = 0
	integer = 0
	string = ""
	numberArray = []
	booleanMap = {}
}

resource child2 "simple:index:Resource" {
	__logicalName = "child2"
	value = true

	options {
		parent = parent2
	}
}

resource child3 "primitive:index:Resource" {
	__logicalName = "child3"
	boolean = false
	float = 0
	integer = 0
	string = ""
	numberArray = []
	booleanMap = {}

	options {
		parent = parent1
	}
}
