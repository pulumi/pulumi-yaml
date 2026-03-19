resource withOption "simple:index:Resource" {
	__logicalName = "withOption"
	value = false

	options {
		deleteBeforeReplace = true
		replaceOnChanges = [value]
	}
}

resource withoutOption "simple:index:Resource" {
	__logicalName = "withoutOption"
	value = false

	options {
		replaceOnChanges = [value]
	}
}
