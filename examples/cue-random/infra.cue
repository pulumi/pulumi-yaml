package pulumi

resources:
	randomPassword: {
		type: "random:RandomPassword"
		properties: {
			length:          16
			special:         true
			overrideSpecial: "_%@"
		}
	}

outputs:
	password: "${randomPassword.result}"
