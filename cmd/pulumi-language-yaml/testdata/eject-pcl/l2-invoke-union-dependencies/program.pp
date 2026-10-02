data = invoke("simple-invoke:index:secretInvoke", {
	value = a.text,
	secretResponse = b.value
})

resource a "simple-invoke:index:StringResource" {
	__logicalName = "a"
	text = "hello"
}

resource b "simple:index:Resource" {
	__logicalName = "b"
	value = true
}

resource d "simple-invoke:index:StringResource" {
	__logicalName = "d"
	text = data.response
}
