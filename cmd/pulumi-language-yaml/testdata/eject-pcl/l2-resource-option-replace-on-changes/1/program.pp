// Stage 1: Change properties to trigger replacements
//
// Scenario 1: Change replaceProp → REPLACE (schema triggers)
resource schemaReplace "replaceonchanges:index:ResourceA" {
	__logicalName = "schemaReplace"
	value = true
	replaceProp = false
}

// Scenario 2: Change value → REPLACE (option triggers)
resource optionReplace "replaceonchanges:index:ResourceB" {
	__logicalName = "optionReplace"
	value = false

	options {
		replaceOnChanges = [value]
	}
}

// Scenario 3: Change value → REPLACE (option on value triggers)
resource bothReplaceValue "replaceonchanges:index:ResourceA" {
	__logicalName = "bothReplaceValue"
	value = false
	replaceProp = true

	options {
		replaceOnChanges = [value]
	}
}

// Scenario 4: Change replaceProp → REPLACE (schema on replaceProp triggers)
resource bothReplaceProp "replaceonchanges:index:ResourceA" {
	__logicalName = "bothReplaceProp"
	value = true
	replaceProp = false

	options {
		replaceOnChanges = [value]
	}
}

// Scenario 5: Change value → UPDATE (no replaceOnChanges)
resource regularUpdate "replaceonchanges:index:ResourceB" {
	__logicalName = "regularUpdate"
	value = false
}

// Scenario 6: No change → SAME (no operation)
resource noChange "replaceonchanges:index:ResourceB" {
	__logicalName = "noChange"
	value = true

	options {
		replaceOnChanges = [value]
	}
}

// Scenario 7: Change replaceProp (not value) → UPDATE (marked property unchanged)
resource wrongPropChange "replaceonchanges:index:ResourceA" {
	__logicalName = "wrongPropChange"
	value = true
	replaceProp = false

	options {
		replaceOnChanges = [value]
	}
}

// Scenario 8: Change value → REPLACE (multiple properties marked)
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
