resource source "bytesource:index:Resource" {
	__logicalName = "source"
	base64 = "AGhlbGxvIID+/yB3b3JsZPAo"
}

resource sink "bytesink:index:Resource" {
	__logicalName = "sink"
	bytes = source.bytes
	expectBase64 = source.base64
}
