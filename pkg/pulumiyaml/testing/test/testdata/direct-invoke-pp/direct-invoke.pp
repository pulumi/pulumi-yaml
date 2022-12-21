resource example "aws:iam:Policy" {
  name   = "example_policy"
  path   = "/"
  tags = { 3 = 4 }
  policy = invoke("aws:iam:getPolicyDocument", {
    statements = [{
      sid = "1"

      actions = [
        "s3:ListAllMyBuckets",
        "s3:GetBucketLocation",
      ]

      resources = [
        "arn:aws:s3:::*",
      ]
    }]
  }).json
}
