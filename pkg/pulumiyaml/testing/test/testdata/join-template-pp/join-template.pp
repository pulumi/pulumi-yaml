resource example "aws:iam:Policy" {
  name   = "${element(["foo","bar"], 0)}-policy"
  path   = "/"
  policy = element(split("{}-foo","-"), 0)
  }
