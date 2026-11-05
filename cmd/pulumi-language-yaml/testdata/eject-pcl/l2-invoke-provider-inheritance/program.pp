parentData = invoke("simple-invoke:index:myInvoke", {
	value = "parent"
}, {
	parent = parent
})
childData = invoke("simple-invoke:index:myInvoke", {
	value = "child"
}, {
	parent = child
})

resource provider "pulumi:providers:simple-invoke" {
	__logicalName = "provider"
}

resource parent "simple-invoke:index:StringResource" {
	__logicalName = "parent"
	text = "parent"

	options {
		provider = provider
	}
}

resource child "simple-invoke:index:StringResource" {
	__logicalName = "child"
	text = "child"

	options {
		parent = parent
	}
}

output parentResult {
	__logicalName = "parentResult"
	value = parentData.result
}

output childResult {
	__logicalName = "childResult"
	value = childData.result
}
