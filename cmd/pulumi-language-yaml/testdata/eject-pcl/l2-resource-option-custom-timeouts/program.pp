resource noTimeouts "simple:index:Resource" {
	__logicalName = "noTimeouts"
	value = true
}

resource createOnly "simple:index:Resource" {
	__logicalName = "createOnly"
	value = true

	options {
		customTimeouts = {
			create = "5m"
		}
	}
}

resource updateOnly "simple:index:Resource" {
	__logicalName = "updateOnly"
	value = true

	options {
		customTimeouts = {
			update = "10m"
		}
	}
}

resource deleteOnly "simple:index:Resource" {
	__logicalName = "deleteOnly"
	value = true

	options {
		customTimeouts = {
			delete = "3m"
		}
	}
}

resource allTimeouts "simple:index:Resource" {
	__logicalName = "allTimeouts"
	value = true

	options {
		customTimeouts = {
			create = "2m",
			update = "4m",
			delete = "1m"
		}
	}
}
