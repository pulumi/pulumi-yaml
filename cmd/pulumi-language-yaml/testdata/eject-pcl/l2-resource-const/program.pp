resource first "constant:index:Resource" {
	__logicalName = "first"
	kind = "Constant"
}

output kind {
	__logicalName = "kind"
	value = first.kind
}
