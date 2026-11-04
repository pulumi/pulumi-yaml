// This provider covers scenarios where configuration properties are marked as secret in the schema.
resource configGrpcProvider "pulumi:providers:config-grpc" {
	__logicalName = "config_grpc_provider"
	secretString1 = "SECRET"
	secretInt1 = 16
	secretNum1 = 123456.789
	secretBool1 = true
	listSecretString1 = [
		"SECRET",
		"SECRET2"
	]
	mapSecretString1 = {
		"key1" = "SECRET",
		"key2" = "SECRET2"
	}
	objSecretString1 = {
		secretX = "SECRET"
	}
}

resource config "config-grpc:index:ConfigFetcher" {
	__logicalName = "config"

	options {
		provider = configGrpcProvider
	}
}
