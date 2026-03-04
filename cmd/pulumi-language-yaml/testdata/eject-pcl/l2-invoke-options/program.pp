data = invoke("simple-invoke:index:myInvoke", {
	value = "hello"
}, {
	parent = explicitProvider,
	provider = explicitProvider,
	version = "10.0.0",
	pluginDownloadUrl = "https://example.com/github/example"
})

resource explicitProvider "pulumi:providers:simple-invoke" {
	__logicalName = "explicitProvider"

	options {
		version = "10.0.0"
		pluginDownloadURL = "https://example.com/github/example"
	}
}

output hello {
	__logicalName = "hello"
	value = data.result
}
