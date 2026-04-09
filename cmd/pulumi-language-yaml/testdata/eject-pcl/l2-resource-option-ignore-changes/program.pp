resource receiverIgnore "nestedobject:index:Receiver" {
	__logicalName = "receiverIgnore"
	details = [{
		key = "a",
		value = "b"
	}]

	options {
		ignoreChanges = [details[0].key]
	}
}

resource mapIgnore "nestedobject:index:MapContainer" {
	__logicalName = "mapIgnore"
	tags = {
		"env" = "prod"
	}

	options {
		ignoreChanges = [
			tags["env"],
			tags["with.dot"],
			tags["with escaped \""]
		]
	}
}

resource noIgnore "nestedobject:index:Target" {
	__logicalName = "noIgnore"
	name = "nothing"
}
