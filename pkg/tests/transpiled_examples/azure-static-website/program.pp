resource staticsitegroup "azure-native:resources:ResourceGroup" {
}

resource storageaccount "azure-native:storage:StorageAccount" {
	resourceGroupName = staticsitegroup.name
	kind = "StorageV2"
	sku = {
		"name" = "Standard_LRS"
	}
}

resource staticwebsite "azure-native:storage:StorageAccountStaticWebsite" {
	resourceGroupName = staticsitegroup.name
	accountName = storageaccount.name
	indexDocument = "index.html"
	error404Document = "404.html"
}

resource indexhtml "azure-native:storage:Blob" {
	resourceGroupName = staticsitegroup.name
	accountName = storageaccount.name
	containerName = staticwebsite.containerName
	contentType = "text/html"
	type = "Block"
	source = fileAsset("./www/index.html")
}

resource faviconpng "azure-native:storage:Blob" {
	resourceGroupName = staticsitegroup.name
	accountName = storageaccount.name
	containerName = staticwebsite.containerName
	contentType = "image/png"
	type = "Block"
	source = fileAsset("./www/favicon.png")
}

resource _404html "azure-native:storage:Blob" {
	resourceGroupName = staticsitegroup.name
	accountName = storageaccount.name
	containerName = staticwebsite.containerName
	contentType = "text/html"
	type = "Block"
	source = fileAsset("./www/404.html")
}

output endpoint {
	value = storageaccount.primaryEndpoints.web
}
