resource classResource "simple:index:Resource" {
	__logicalName = "class"
	value = true
}

resource exportResource "simple:index:Resource" {
	__logicalName = "export"
	value = true
}

resource modResource "simple:index:Resource" {
	__logicalName = "mod"
	value = true
}

resource import "simple:index:Resource" {
	__logicalName = "import"
	value = true
}

resource objectResource "simple:index:Resource" {
	__logicalName = "object"
	value = true
}

resource selfResource "simple:index:Resource" {
	__logicalName = "self"
	value = true
}

resource thisResource "simple:index:Resource" {
	__logicalName = "this"
	value = true
}

resource ifResource "simple:index:Resource" {
	__logicalName = "if"
	value = true
}

output class {
	__logicalName = "class"
	value = classResource
}

output export {
	__logicalName = "export"
	value = exportResource
}

output mod {
	__logicalName = "mod"
	value = modResource
}

output object {
	__logicalName = "object"
	value = objectResource
}

output self {
	__logicalName = "self"
	value = selfResource
}

output this {
	__logicalName = "this"
	value = thisResource
}

output if {
	__logicalName = "if"
	value = ifResource
}
