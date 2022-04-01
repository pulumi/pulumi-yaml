package main

import "examples.pulumi.com/yaml-eks/aws:eks"

resources: {
	rawkode: eks.#EksCluster
	stack72: eks.#EksCluster & {
		properties: {
			instanceType:    "rawkode"
			desiredCapacity: 4
		}
	}
}
