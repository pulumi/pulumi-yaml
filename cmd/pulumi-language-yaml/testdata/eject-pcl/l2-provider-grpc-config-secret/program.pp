resource configGrpcProvider "pulumi:providers:config-grpc" {
	__logicalName = "config_grpc_provider"
	string1 = invoke("config-grpc:index:toSecret", {
		string1 = "SECRET"
	}).string1
	int1 = invoke("config-grpc:index:toSecret", {
		int1 = 1234567890
	}).int1
	num1 = invoke("config-grpc:index:toSecret", {
		num1 = 123456.789
	}).num1
	bool1 = invoke("config-grpc:index:toSecret", {
		bool1 = true
	}).bool1
	listString1 = invoke("config-grpc:index:toSecret", {
		listString1 = [
			"SECRET",
			"SECRET2"
		]
	}).listString1
	listString2 = [
		"VALUE",
		invoke("config-grpc:index:toSecret", {
			string1 = "SECRET"
		}).string1
	]
	mapString2 = {
		"key1" = "value1",
		"key2" = invoke("config-grpc:index:toSecret", {
			string1 = "SECRET"
		}).string1
	}
	objString2 = {
		x = invoke("config-grpc:index:toSecret", {
			string1 = "SECRET"
		}).string1
	}
}

resource config "config-grpc:index:ConfigFetcher" {
	__logicalName = "config"

	options {
		provider = configGrpcProvider
	}
}
