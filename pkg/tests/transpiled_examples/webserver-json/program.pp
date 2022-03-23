config instanceType string {
	default = "t3.micro"
}

resource webSecGrp "aws:ec2/securityGroup:SecurityGroup" {
	ingress = [{
		protocol = "tcp",
		fromPort = 80,
		toPort = 80,
		cidrBlocks = ["0.0.0.0/0"]
	}]
}

resource webServer "aws:ec2/instance:Instance" {
	instanceType = instanceType
	ami = invoke("aws:index/getAmi:getAmi", {
		filters = [{
			name = "name",
			values = ["amzn-ami-hvm-*-x86_64-ebs"]
		}],
		owners = ["137112412989"],
		mostRecent = true
	}).id
	userData = join("\n", [
		"#!/bin/bash",
		"echo 'Hello, World from ${webSecGrp.arn}!' > index.html",
		"nohup python -m SimpleHTTPServer 80 &"
	])
	vpcSecurityGroupIds = [webSecGrp.id]
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
