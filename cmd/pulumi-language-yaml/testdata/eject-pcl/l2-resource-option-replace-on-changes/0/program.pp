resource schemaReplace "replaceonchanges:index:ResourceA" {
	__logicalName = "schemaReplace"
	value = true
	replaceProp = true
}

resource optionReplace "replaceonchanges:index:ResourceB" {
	__logicalName = "optionReplace"
	value = true

	options {
		replaceOnChanges = [value]
	}
}

resource bothReplaceValue "replaceonchanges:index:ResourceA" {
	__logicalName = "bothReplaceValue"
	value = true
	replaceProp = true

	options {
		replaceOnChanges = [value]
	}
}

resource bothReplaceProp "replaceonchanges:index:ResourceA" {
	__logicalName = "bothReplaceProp"
	value = true
	replaceProp = true

	options {
		replaceOnChanges = [value]
	}
}

resource regularUpdate "replaceonchanges:index:ResourceB" {
	__logicalName = "regularUpdate"
	value = true
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
	replaceProp = true

	options {
		replaceOnChanges = [value]
	}
}

resource multiplePropReplace "replaceonchanges:index:ResourceA" {
	__logicalName = "multiplePropReplace"
	value = true
	replaceProp = true

	options {
		replaceOnChanges = [
			value,
			replaceProp
		]
	}
}
