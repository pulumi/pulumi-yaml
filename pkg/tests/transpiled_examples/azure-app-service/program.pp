config sqlAdmin string {
	default = "pulumi"
}

blobAccessToken = invoke("azure-native:storage:listStorageAccountServiceSAS", {
	accountName = sa.name,
	protocols = "https",
	sharedAccessStartTime = "2022-01-01",
	sharedAccessExpiryTime = "2030-01-01",
	resource = "c",
	resourceGroupName = appservicegroup.name,
	permissions = "r",
	canonicalizedResource = "/blob/${sa.name}/${container.name}",
	contentType = "application/json",
	cacheControl = "max-age=5",
	contentDisposition = "inline",
	contentEncoding = "deflate"
}).serviceSasToken

resource appservicegroup "azure-native:resources:ResourceGroup" {
}

resource sa "azure-native:storage:StorageAccount" {
	resourceGroupName = appservicegroup.name
	kind = "StorageV2"
	sku = {
		name = "Standard_LRS"
	}
}

resource appserviceplan "azure-native:web:AppServicePlan" {
	resourceGroupName = appservicegroup.name
	kind = "App"
	sku = {
		name = "B1",
		tier = "Basic"
	}
}

resource container "azure-native:storage:BlobContainer" {
	resourceGroupName = appservicegroup.name
	accountName = sa.name
	publicAccess = "None"
}

resource blob "azure-native:storage:Blob" {
	resourceGroupName = appservicegroup.name
	accountName = sa.name
	containerName = container.name
	type = "Block"
	source = fileArchive("./www")
}

resource appInsights "azure-native:insights:Component" {
	resourceGroupName = appservicegroup.name
	applicationType = "web"
	kind = "web"
}

resource sqlPassword "random:index/randomPassword:RandomPassword" {
	length = 16
	special = true
}

resource sqlServer "azure-native:sql:Server" {
	resourceGroupName = appservicegroup.name
	administratorLogin = sqlAdmin
	administratorLoginPassword = sqlPassword.result
	version = "12.0"
}

resource db "azure-native:sql:Database" {
	resourceGroupName = appservicegroup.name
	serverName = sqlServer.name
	sku = {
		name = "S0"
	}
}

resource app "azure-native:web:WebApp" {
	resourceGroupName = appservicegroup.name
	serverFarmId = appserviceplan.id
	siteConfig = {
		appSettings = [
			{
				name = "WEBSITE_RUN_FROM_PACKAGE",
				value = "https://${sa.name}.blob.core.windows.net/${container.name}/${blob.name}?${blobAccessToken}"
			},
			{
				name = "APPINSIGHTS_INSTRUMENTATIONKEY",
				value = appInsights.instrumentationKey
			},
			{
				name = "APPLICATIONINSIGHTS_CONNECTION_STRING",
				value = "InstrumentationKey=${appInsights.instrumentationKey}"
			},
			{
				name = "ApplicationInsightsAgent_EXTENSION_VERSION",
				value = "~2"
			}
		],
		connectionStrings = [{
			name = "db",
			type = "SQLAzure",
			connectionString = "Server= tcp:${sqlServer.name}.database.windows.net;initial catalog=${db.name};userID=${sqlAdmin};password=${sqlPassword.result};Min Pool Size=0;Max Pool Size=30;Persist Security Info=true;"
		}]
	}
}

output endpoint {
	value = app.defaultHostName
}
