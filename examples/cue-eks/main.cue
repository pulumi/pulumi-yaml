package main

import "examples.pulumi.com/yaml-eks/aws:eks"

resources: {
	rawkode: eks.#EksCluster
}
