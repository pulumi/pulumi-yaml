resource parent "simple:index:Resource" {
	__logicalName = "parent"
	value = true
}

resource aliasURN "simple:index:Resource" {
	__logicalName = "aliasURN"
	value = true

	options {
		aliases = ["urn:pulumi:test::l2-resource-option-alias::simple:index:Resource::aliasURN"]
		parent = parent
	}
}

resource aliasNewName "simple:index:Resource" {
	__logicalName = "aliasNewName"
	value = true

	options {
		aliases = [{
			name = "aliasName"
		}]
	}
}

resource aliasNoParent "simple:index:Resource" {
	__logicalName = "aliasNoParent"
	value = true

	options {
		aliases = [{
			noParent = true
		}]
		parent = parent
	}
}

resource aliasParent "simple:index:Resource" {
	__logicalName = "aliasParent"
	value = true

	options {
		aliases = [{
			parent = aliasURN
		}]
		parent = parent
	}
}
