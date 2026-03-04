resource res "simple:index:Resource" {
	__logicalName = "res"
	value = true
}

output nonSecret {
	__logicalName = "nonSecret"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = "hello",
		secretResponse = false
	}).response
}

output firstSecret {
	__logicalName = "firstSecret"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = "hello",
		secretResponse = res.value
	}).response
}

output secondSecret {
	__logicalName = "secondSecret"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = secret("goodbye"),
		secretResponse = false
	}).response
}
