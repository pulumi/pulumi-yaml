config instanceType string {
	default = "t3.micro"
}

ec2ami = invoke("aws:index/getAmi:getAmi", {
	"filters" = [{
		"name" = "name",
		"values" = ["amzn-ami-hvm-*-x86_64-ebs"]
	}],
	"owners" = ["137112412989"],
	"mostRecent" = true
}).id

resource webSecGrp "aws:ec2/securityGroup:SecurityGroup" {
	ingress = [{
		"protocol" = "tcp",
		"fromPort" = 80,
		"toPort" = 80,
		"cidrBlocks" = ["0.0.0.0/0"]
	}]
}

resource webServer "aws:ec2/instance:Instance" {
	instanceType = instanceType
	ami = ec2ami
	userData = "#!/bin/bash\necho 'Hello, World from ${webSecGrp.arn}!' > index.html\nnohup python -m SimpleHTTPServer 80 &"
	vpcSecurityGroupIds = [webSecGrp.id]
}

resource usEast2Provider "pulumi:providers:aws" {
	region = "us-east-2"
}

resource myBucket "aws:s3/bucket:Bucket" {
	options {
		provider = usEast2Provider
	}
}

output instanceId {
	value = webServer.id
}

output publicIp {
	value = webServer.publicIp
}

output publicHostName {
	value = webServer.publicDns
}
