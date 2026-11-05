resource a "output-value-input-deps:index:Custom" {
	__logicalName = "a"
	value = "a"
}

resource b "output-value-input-deps:index:Custom" {
	__logicalName = "b"
	value = "b"
}

resource comp "output-value-input-deps:index:Component" {
	__logicalName = "comp"
	refs = {
		x = a.value,
		y = b.value
	}
}
