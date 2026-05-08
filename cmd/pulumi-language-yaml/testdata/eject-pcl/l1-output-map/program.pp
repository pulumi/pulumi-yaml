output empty {
	__logicalName = "empty"
	value = {}
}

output strings {
	__logicalName = "strings"
	value = {
		"greeting" = "Hello, world!",
		"farewell" = "Goodbye, world!"
	}
}

output adversarialStrings {
	__logicalName = "adversarialStrings"
	value = {
		"__type" = "dunder type",
		"__internal" = "dunder internal",
		"__provider" = "dunder provider",
		"__version" = "dunder version",
		"" = "empty key",
		"empty value" = "",
		"dunder value" = "__dunder",
		"Some $${common} \"characters\" 'that' need escaping: \\ (backslash), \t (tab), \u001b (escape), \u0007 (bell), \u0000 (null), \U000e0021 (tag space)" = "Some $${common} \"characters\" 'that' need escaping: \\ (backslash), \t (tab), \u001b (escape), \u0007 (bell), \u0000 (null), \U000e0021 (tag space)"
	}
}

output numbers {
	__logicalName = "numbers"
	value = {
		"1" = 1,
		"2" = 2
	}
}

output keys {
	__logicalName = "keys"
	value = {
		"my.key" = 1,
		"my-key" = 2,
		"my_key" = 3,
		"MY_KEY" = 4,
		"mykey" = 5,
		"MYKEY" = 6
	}
}
