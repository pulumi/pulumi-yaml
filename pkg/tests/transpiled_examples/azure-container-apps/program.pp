config sqlAdmin string {
	default = "pulumi"
}

sharedKey = invoke("azure-native:operationalinsights:getSharedKeys", {
	"resourceGroupName" = resourceGroup.name,
	"workspaceName" = workspace.name
}).primarySharedKey
adminUsername = invoke("azure-native:containerregistry:listRegistryCredentials", {
	"resourceGroupName" = resourceGroup.name,
	"registryName" = registry.name
}).username
adminPasswords = invoke("azure-native:containerregistry:listRegistryCredentials", {
	"resourceGroupName" = resourceGroup.name,
	"registryName" = registry.name
}).passwords

resource resourceGroup "azure-native:resources:ResourceGroup" {
}

resource workspace "azure-native:operationalinsights:Workspace" {
	resourceGroupName = resourceGroup.name
	sku = {
		name = "PerGB2018"
	}
	retentionInDays = 30
}

resource kubeEnv "azure-native:web:KubeEnvironment" {
	resourceGroupName = resourceGroup.name
	environmentType = "Managed"
	appLogsConfiguration = {
		destination = "log-analytics",
		logAnalyticsConfiguration = {
			"customerId" = workspace.customerId,
			"sharedKey" = sharedKey
		}
	}
}

resource registry "azure-native:containerregistry:Registry" {
	resourceGroupName = resourceGroup.name
	sku = {
		name = "Basic"
	}
	adminUserEnabled = true
}

resource provider "pulumi:providers:docker" {
	registryAuth = [{
		"address" = registry.loginServer,
		"username" = adminUsername,
		"password" = adminPasswords[0].value
	}]
}

resource myImage "docker:index/registryImage:RegistryImage" {
	name = "${registry.loginServer}/node-app:v1.0.0"
	build = {
		context = "${cwd()}/node-app"
	}

	options {
		provider = provider
	}
}

resource containerapp "azure-native:web:ContainerApp" {
	resourceGroupName = resourceGroup.name
	kubeEnvironmentId = kubeEnv.id
	configuration = {
		ingress = {
			"external" = true,
			"targetPort" = 80
		},
		registries = [{
			"server" = registry.loginServer,
			"username" = adminUsername,
			"passwordSecretRef" = "pwd"
		}],
		secrets = [{
			"name" = "pwd",
			"value" = adminPasswords[0].value
		}]
	}
	template = {
		containers = [{
			"name" = "myapp",
			"image" = myImage.name
		}]
	}
}

output endpoint {
	value = "https://${containerapp.configuration.ingress.fqdn}"
}
