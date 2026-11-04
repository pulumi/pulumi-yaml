config configLexicalName bool {
	__logicalName = "cC-Charlie_charlie.\U0001f603⁉️"
}

resource resourceLexicalName "simple:index:Resource" {
	__logicalName = "aA-Alpha_alpha.\U0001f92f⁉️"
	value = configLexicalName
}

output bBBetaBeta {
	__logicalName = "bB-Beta_beta.\U0001f49c⁉"
	value = resourceLexicalName.value
}

// New format for output logical name because outputs don't have separate logical names. Even nodejs which just
// does "export" normally for outputs needs that export _to be_ the output name and so if the "logical name"
// isn't a valid nodejs export we have to output it differently.
output dDDeltaDelta {
	__logicalName = "dD-Delta_delta.\U0001f525⁉"
	value = resourceLexicalName.value
}
