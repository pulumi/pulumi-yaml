config aList "list(string)" {
	__logicalName = "aList"
}

config singleOrNoneList "list(string)" {
	__logicalName = "singleOrNoneList"
}

config aString string {
	__logicalName = "aString"
}

output elementOutput1 {
	__logicalName = "elementOutput1"
	value = aList[1]
}

output elementOutput2 {
	__logicalName = "elementOutput2"
	value = aList[2]
}

output joinOutput {
	__logicalName = "joinOutput"
	value = join("|", aList)
}

output lengthOutput {
	__logicalName = "lengthOutput"
	value = length(aList)
}

output splitOutput {
	__logicalName = "splitOutput"
	value = split("-", aString)
}

output singleOrNoneOutput {
	__logicalName = "singleOrNoneOutput"
	value = [singleOrNone(singleOrNoneList)]
}
