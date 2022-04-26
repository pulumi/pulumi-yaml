config hostname string {
	default = "example.com"
}

resource nginxdemo "kubernetes:core/v1:Namespace" {
}

resource app "kubernetes:apps/v1:Deployment" {
	metadata = {
		namespace = nginxdemo.metadata.name
	}
	spec = {
		selector = {
			matchLabels = {
				"app.kubernetes.io/name" = "nginx-demo"
			}
		},
		replicas = 1,
		template = {
			metadata = {
				labels = {
					"app.kubernetes.io/name" = "nginx-demo"
				}
			},
			spec = {
				containers = [{
					name = "app",
					image = "nginx:1.15-alpine"
				}]
			}
		}
	}
}

resource service "kubernetes:core/v1:Service" {
	metadata = {
		namespace = nginxdemo.metadata.name,
		labels = {
			"app.kubernetes.io/name" = "nginx-demo"
		}
	}
	spec = {
		type = "ClusterIP",
		ports = [{
			port = 80,
			targetPort = 80,
			protocol = "TCP"
		}],
		selector = {
			"app.kubernetes.io/name" = "nginx-demo"
		}
	}
}

resource ingress "kubernetes:networking.k8s.io/v1:Ingress" {
	metadata = {
		namespace = nginxdemo.metadata.name
	}
	spec = {
		rules = [{
			host = hostname,
			http = {
				paths = [{
					path = "/",
					pathType = "Prefix",
					backend = {
						service = {
							name = service.metadata.name,
							port = {
								number = 80
							}
						}
					}
				}]
			}
		}]
	}
}
