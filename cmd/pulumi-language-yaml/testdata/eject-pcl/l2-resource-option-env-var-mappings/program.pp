resource prov "pulumi:providers:simple" {
	__logicalName = "prov"

	options {
		envVarMappings = {
			"MY_VAR" = "PROVIDER_VAR",
			"OTHER_VAR" = "TARGET_VAR"
		}
	}
}
