resource res "simple:index:Resource" {
	__logicalName = "res"
	value = true
}

// inputs are plain and the invoke response is plain
output nonSecret {
	__logicalName = "nonSecret"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = "hello",
		secretResponse = false
	}).response
}

// referencing value from resource
// invoke response is secret => whole output is secret
output firstSecret {
	__logicalName = "firstSecret"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = "hello",
		secretResponse = res.value
	}).response
}

// inputs are secret, invoke response is plain => whole output is secret
output secondSecret {
	__logicalName = "secondSecret"
	value = invoke("simple-invoke:index:secretInvoke", {
		value = secret("goodbye"),
		secretResponse = false
	}).response
}
