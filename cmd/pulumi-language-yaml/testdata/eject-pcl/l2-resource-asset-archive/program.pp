resource ass "asset-archive:index:AssetResource" {
	__logicalName = "ass"
	value = fileAsset("../test.txt")
}

resource arc "asset-archive:index:ArchiveResource" {
	__logicalName = "arc"
	value = fileArchive("../archive.tar")
}

resource dir "asset-archive:index:ArchiveResource" {
	__logicalName = "dir"
	value = fileArchive("../folder")
}

resource assarc "asset-archive:index:ArchiveResource" {
	__logicalName = "assarc"
	value = assetArchive({
		"string" = stringAsset("file contents"),
		"file" = fileAsset("../test.txt"),
		"folder" = fileArchive("../folder"),
		"archive" = fileArchive("../archive.tar")
	})
}

resource remoteass "asset-archive:index:AssetResource" {
	__logicalName = "remoteass"
	value = remoteAsset("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/test.txt")
}

resource remotearc "asset-archive:index:ArchiveResource" {
	__logicalName = "remotearc"
	value = remoteArchive("https://raw.githubusercontent.com/pulumi/pulumi/7b0eb7fb10694da2f31c0d15edf671df843e0d4c/cmd/pulumi-test-language/tests/testdata/l2-resource-asset-archive/archive.tar")
}

output assetOutput {
	__logicalName = "assetOutput"
	value = fileAsset("../test.txt")
}

output archiveOutput {
	__logicalName = "archiveOutput"
	value = fileArchive("../archive.tar")
}

output assetList {
	__logicalName = "assetList"
	value = [
		fileAsset("../test.txt"),
		stringAsset("file contents")
	]
}

output archiveList {
	__logicalName = "archiveList"
	value = [
		fileArchive("../archive.tar"),
		fileArchive("../folder")
	]
}

output assetMap {
	__logicalName = "assetMap"
	value = {
		"file" = fileAsset("../test.txt"),
		"string" = stringAsset("file contents")
	}
}

output archiveMap {
	__logicalName = "archiveMap"
	value = {
		"tar" = fileArchive("../archive.tar"),
		"folder" = fileArchive("../folder")
	}
}
