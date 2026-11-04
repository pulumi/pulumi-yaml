// Stage 1: Change properties to trigger replacements
//
// Resource with deleteBeforeReplace option - should delete before creating
resource withOption "simple:index:Resource" {
	__logicalName = "withOption"
	value = false

	options {
		deleteBeforeReplace = true
		replaceOnChanges = [value]
	}
}

// Resource without deleteBeforeReplace - should create before deleting (default)
resource withoutOption "simple:index:Resource" {
	__logicalName = "withoutOption"
	value = false

	options {
		replaceOnChanges = [value]
	}
}
