config anObject "map(any)" {
	__logicalName = "anObject"
}

config anyObject "map(any)" {
	__logicalName = "anyObject"
}

lVar = secret([1])
mVar = secret({
	"key" = true
})
cVar = secret(anObject)
oVar = secret({
	"property" = "value"
})
aVar = secret(anyObject)

output l {
	__logicalName = "l"
	value = lVar[0]
}

output m {
	__logicalName = "m"
	value = mVar.key
}

output c {
	__logicalName = "c"
	value = cVar.property
}

output o {
	__logicalName = "o"
	value = oVar.property
}

output a {
	__logicalName = "a"
	value = aVar.property
}
