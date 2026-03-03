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
