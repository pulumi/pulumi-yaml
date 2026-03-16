config configLexicalName bool {
	__logicalName = "configLexicalName"
}

resource resourceLexicalName "simple:index:Resource" {
	__logicalName = "resourceLexicalName"
	value = configLexicalName
}

output bBBetaBeta {
	__logicalName = "bB-Beta_beta.💜⁉"
	value = resourceLexicalName.value
}

output dDDeltaDelta {
	__logicalName = "dD-Delta_delta.🔥⁉"
	value = resourceLexicalName.value
}
