data = invoke("simple-invoke:index:secretInvoke", {
	value = a.text,
	secretResponse = b.value
})

// Baseline for invoke dependency propagation: an invoke that reads properties from two different
// resources produces a return value whose consumer must depend on the union of both.
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
