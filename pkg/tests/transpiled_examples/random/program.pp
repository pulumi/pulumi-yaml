resource randomPassword "random:index/randomPassword:RandomPassword" {
	length = 16
	special = true
	overrideSpecial = "_%@"
}

output password {
	value = randomPassword.result
}
