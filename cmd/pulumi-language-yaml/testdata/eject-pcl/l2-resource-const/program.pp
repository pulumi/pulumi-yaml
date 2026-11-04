resource first "constant:index:Resource" {
	__logicalName = "first"
	kind = "Constant"
	flag = true
	count = 3
	ratio = 1.5
}

// Every property has a constant value in the schema, one per constant kind; reading them must
// bind without type errors.
output kind {
	__logicalName = "kind"
	value = first.kind
}

output flag {
	__logicalName = "flag"
	value = first.flag
}

output count {
	__logicalName = "count"
	value = first.count
}

output ratio {
	__logicalName = "ratio"
	value = first.ratio
}
