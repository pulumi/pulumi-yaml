resource res "secret:index:Resource" {
	__logicalName = "res"
	private = "closed"
	public = "open"
	privateData = {
		private = "closed",
		public = "open"
	}
	publicData = {
		private = "closed",
		public = "open"
	}
}
