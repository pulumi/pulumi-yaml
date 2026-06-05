data = invoke("simple-invoke:index:myInvoke", {
	value = "hello"
}, {
	dependsOn = [first]
})

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
