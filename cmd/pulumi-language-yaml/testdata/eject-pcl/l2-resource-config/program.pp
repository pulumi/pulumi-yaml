resource prov "pulumi:providers:config" {
	__logicalName = "prov"
	name = "my config"
	pluginDownloadURL = "not the same as the pulumi resource option"
}

resource res "config:index:Resource" {
	__logicalName = "res"
	text = prov.version
}

output pluginDownloadURL {
	__logicalName = "pluginDownloadURL"
	value = prov.pluginDownloadURL
}
