data = invoke("simple-invoke:index:myInvoke", {
	value = "hello"
}, {
	dependsOn = [first]
})

resource explicitProvider "pulumi:providers:simple-invoke" {
	__logicalName = "explicitProvider"
}

resource first "simple-invoke:index:StringResource" {
	__logicalName = "first"
	text = "first hello"
}

resource second "simple-invoke:index:StringResource" {
	__logicalName = "second"
	text = data.result
}

output hello {
	__logicalName = "hello"
	value = data.result
}
