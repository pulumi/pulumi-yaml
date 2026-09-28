resource root "selfref:index:Node" {
	__logicalName = "root"
}

resource child "selfref:index:Node" {
	__logicalName = "child"
	parent = root
	parents = [root]
	namedParents = {
		"root" = root
	}
	parentOrName = root
}
