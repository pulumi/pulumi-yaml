package eks

#EksCluster: {
	type: "eks:Cluster"
	properties: {
		instanceType:    "t2.medium"
		desiredCapacity: 2
		minSize:         1
		maxSize:         2
	}
}
