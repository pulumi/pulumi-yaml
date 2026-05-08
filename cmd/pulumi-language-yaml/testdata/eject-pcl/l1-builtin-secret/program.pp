config aSecret string {
	__logicalName = "aSecret"
	secret = true
}

config notSecret string {
	__logicalName = "notSecret"
}

output roundtripSecret {
	__logicalName = "roundtripSecret"
	value = aSecret
}

output roundtripNotSecret {
	__logicalName = "roundtripNotSecret"
	value = notSecret
}

output double {
	__logicalName = "double"
	value = secret(aSecret)
}

output open {
	__logicalName = "open"
	value = unsecret(aSecret)
}

output close {
	__logicalName = "close"
	value = secret(notSecret)
}
