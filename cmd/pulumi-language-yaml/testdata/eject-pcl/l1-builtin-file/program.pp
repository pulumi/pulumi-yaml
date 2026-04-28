fileContentVar = readFile("testfile.txt")
fileB64Var = filebase64("testfile.txt")
fileShaVar = filebase64sha256("testfile.txt")

output fileContent {
	__logicalName = "fileContent"
	value = fileContentVar
}

output fileB64 {
	__logicalName = "fileB64"
	value = fileB64Var
}

output fileSha {
	__logicalName = "fileSha"
	value = fileShaVar
}
