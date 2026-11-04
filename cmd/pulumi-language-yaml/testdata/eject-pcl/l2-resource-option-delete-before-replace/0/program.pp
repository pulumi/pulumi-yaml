// Stage 0: Initial resource creation
//
// Resource with deleteBeforeReplace option
resource withOption "simple:index:Resource" {
	__logicalName = "withOption"
	value = true

	options {
		deleteBeforeReplace = true
		replaceOnChanges = [value]
	}
}

// Resource without deleteBeforeReplace (default create-before-delete behavior)
resource withoutOption "simple:index:Resource" {
	__logicalName = "withoutOption"
	value = true

	options {
		replaceOnChanges = [value]
	}
}
