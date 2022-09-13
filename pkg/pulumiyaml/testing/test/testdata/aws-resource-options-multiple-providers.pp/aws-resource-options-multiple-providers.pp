resource provider "pulumi:providers:aws" {
	region = "us-west-2"
}

resource bucket1 "aws:s3:Bucket" {
	options {
		version = 5.13.0
		dependsOn = [provider]
		protect = true
		ignoreChanges = [bucket, lifecycleRules[0]]
	}
}

resource bucket2 "aws:s3:Bucket" {
	options {
		version = 5.13.0
	}
}

resource bucket3 "aws:s3:Bucket" {
	options {
		version = 5.13.0
	}
}

resource thirdPartyObject "scaleway:ObjectBucket" {
	options {
		pluginDownloadURL = github://api.github.com/lbrlabs/pulumi-scaleway
	}
}
