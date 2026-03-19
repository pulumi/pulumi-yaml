resource withOption "simple:index:Resource" {
	__logicalName = "withOption"
	value = true

	options {
		deleteBeforeReplace = true
		replaceOnChanges = [value]
	}
}

resource withoutOption "simple:index:Resource" {
	__logicalName = "withoutOption"
	value = true

	options {
		replaceOnChanges = [value]
	}
}
