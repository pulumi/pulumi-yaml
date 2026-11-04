resource prov "pulumi:providers:config" {
	__logicalName = "prov"
	name = "my config"
	pluginDownloadURL = "not the same as the pulumi resource option"
}

// Note this isn't _using_ the explicit provider, it's just grabbing a value from it.
resource res "config:index:Resource" {
	__logicalName = "res"
	text = prov.version
}

output pluginDownloadUrl {
	__logicalName = "pluginDownloadURL"
	value = prov.pluginDownloadURL
}
