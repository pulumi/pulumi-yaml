resource prov "pulumi:providers:simple" {
	__logicalName = "prov"
}

resource res "simple:index:Resource" {
	__logicalName = "res"
	value = true

	options {
		provider = prov
	}
}
