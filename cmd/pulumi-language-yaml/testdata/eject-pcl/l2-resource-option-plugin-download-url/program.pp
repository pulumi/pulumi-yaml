resource withDefaultURL "simple:index:Resource" {
	__logicalName = "withDefaultURL"
	value = true
}

resource withExplicitDefaultURL "simple:index:Resource" {
	__logicalName = "withExplicitDefaultURL"
	value = true

	options {
		pluginDownloadURL = "https://github.com/pulumi/pulumi-simple/releases/v$${VERSION}"
	}
}

resource withCustomURL1 "simple:index:Resource" {
	__logicalName = "withCustomURL1"
	value = true

	options {
		pluginDownloadURL = "https://custom.pulumi.test/provider1"
	}
}

resource withCustomURL2 "simple:index:Resource" {
	__logicalName = "withCustomURL2"
	value = false

	options {
		pluginDownloadURL = "https://custom.pulumi.test/provider2"
	}
}
