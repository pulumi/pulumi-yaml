resource sitebucket "aws-native:s3:Bucket" {
	websiteConfiguration = {
		indexDocument = "index.html"
	}
}

resource indexhtml "aws:s3/bucketObject:BucketObject" {
	bucket = sitebucket.id
	source = fileAsset("./www/index.html")
	acl = "public-read"
	contentType = "text/html"
}

resource faviconpng "aws:s3/bucketObject:BucketObject" {
	bucket = sitebucket.id
	source = fileAsset("./www/favicon.png")
	acl = "public-read"
	contentType = "image/png"
}

resource bucketPolicy "aws:s3/bucketPolicy:BucketPolicy" {
	bucket = sitebucket.id
	policy = "{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": \"*\",\n      \"Action\": [\"s3:GetObject\"],\n      \"Resource\": [\"${sitebucket.arn}/*\"]\n    }\n  ]\n}\n"
}

output bucketName {
	value = sitebucket.bucketName
}

output websiteUrl {
	value = sitebucket.websiteURL
}
