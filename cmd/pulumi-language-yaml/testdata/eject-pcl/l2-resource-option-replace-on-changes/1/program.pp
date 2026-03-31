resource schemaReplace "replaceonchanges:index:ResourceA" {
	__logicalName = "schemaReplace"
	value = true
	replaceProp = false
}

resource optionReplace "replaceonchanges:index:ResourceB" {
	__logicalName = "optionReplace"
	value = false

	options {
		replaceOnChanges = [value]
	}
}

resource bothReplaceValue "replaceonchanges:index:ResourceA" {
	__logicalName = "bothReplaceValue"
	value = false
	replaceProp = true

	options {
		replaceOnChanges = [value]
	}
}

resource bothReplaceProp "replaceonchanges:index:ResourceA" {
	__logicalName = "bothReplaceProp"
	value = true
	replaceProp = false

	options {
		replaceOnChanges = [value]
	}
}

resource regularUpdate "replaceonchanges:index:ResourceB" {
	__logicalName = "regularUpdate"
	value = false
}

resource noChange "replaceonchanges:index:ResourceB" {
	__logicalName = "noChange"
	value = true

	options {
		replaceOnChanges = [value]
	}
}

resource wrongPropChange "replaceonchanges:index:ResourceA" {
	__logicalName = "wrongPropChange"
	value = true
	replaceProp = false

	options {
		replaceOnChanges = [value]
	}
}

resource multiplePropReplace "replaceonchanges:index:ResourceA" {
	__logicalName = "multiplePropReplace"
	value = false
	replaceProp = true

	options {
		replaceOnChanges = [
			value,
			replaceProp
		]
	}
}
