import pulumi
import pulumi_aws as aws
import pulumi_aws_native as aws_native

sitebucket = aws_native.s3.Bucket("sitebucket", website_configuration=aws_native.s3.BucketWebsiteConfigurationArgs(
    index_document="index.html",
))
indexhtml = aws.s3.BucketObject("indexhtml",
    bucket=sitebucket.id,
    source=pulumi.FileAsset("./www/index.html"),
    acl="public-read",
    content_type="text/html")
faviconpng = aws.s3.BucketObject("faviconpng",
    bucket=sitebucket.id,
    source=pulumi.FileAsset("./www/favicon.png"),
    acl="public-read",
    content_type="image/png")
bucket_policy = aws.s3.BucketPolicy("bucketPolicy",
    bucket=sitebucket.id,
    policy=sitebucket.arn.apply(lambda arn: f"""{{
  "Version": "2012-10-17",
  "Statement": [
    {{
      "Effect": "Allow",
      "Principal": "*",
      "Action": ["s3:GetObject"],
      "Resource": ["{arn}/*"]
    }}
  ]
}}
"""))
pulumi.export("bucketName", sitebucket.bucket_name)
pulumi.export("websiteUrl", sitebucket.website_url)
