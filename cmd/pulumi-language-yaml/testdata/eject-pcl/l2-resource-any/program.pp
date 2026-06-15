resource aString "any-handled:index:Resource" {
	__logicalName = "aString"
	value = "a string"
}

resource aBoolean "any-handled:index:Resource" {
	__logicalName = "aBoolean"
	value = true
}

resource aNumber "any-handled:index:Resource" {
	__logicalName = "aNumber"
	value = 42
}

resource aList "any-handled:index:Resource" {
	__logicalName = "aList"
	value = [
		1,
		true,
		"three"
	]
}

resource anObject "any-handled:index:Resource" {
	__logicalName = "anObject"
	value = {
		"key" = "value",
		"nested" = {
			"count" = 1
		}
	}
}

resource anAsset "any-handled:index:Resource" {
	__logicalName = "anAsset"
	value = stringAsset("the asset contents")
}
