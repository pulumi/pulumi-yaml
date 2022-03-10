using Pulumi;
using Aws = Pulumi.Aws;
using AwsNative = Pulumi.AwsNative;

class MyStack : Stack
{
    public MyStack()
    {
        var sitebucket = new AwsNative.S3.Bucket("sitebucket", new AwsNative.S3.BucketArgs
        {
            WebsiteConfiguration = new AwsNative.S3.Inputs.BucketWebsiteConfigurationArgs
            {
                %!v(PANIC=Format method: interface conversion: model.Expression is *model.TemplateExpression, not *model.LiteralValueExpression),
            });
            var indexhtml = new Aws.S3.BucketObject("indexhtml", new Aws.S3.BucketObjectArgs
            {
                Bucket = sitebucket.Id,
                Source = new FileAsset("./www/index.html"),
                Acl = "public-read",
                ContentType = "text/html",
            });
            var faviconpng = new Aws.S3.BucketObject("faviconpng", new Aws.S3.BucketObjectArgs
            {
                Bucket = sitebucket.Id,
                Source = new FileAsset("./www/favicon.png"),
                Acl = "public-read",
                ContentType = "image/png",
            });
            var bucketPolicy = new Aws.S3.BucketPolicy("bucketPolicy", new Aws.S3.BucketPolicyArgs
            {
                Bucket = sitebucket.Id,
                Policy = sitebucket.Arn.Apply(arn => @$"{{
  ""Version"": ""2012-10-17"",
  ""Statement"": [
    {{
      ""Effect"": ""Allow"",
      ""Principal"": ""*"",
      ""Action"": [""s3:GetObject""],
      ""Resource"": [""{arn}/*""]
    }}
  ]
}}
"),
            });
            this.BucketName = sitebucket.BucketName;
            this.WebsiteUrl = sitebucket.WebsiteURL;
        }

        [Output("bucketName")]
        public Output<string> BucketName { get; set; }
        [Output("websiteUrl")]
        public Output<string> WebsiteUrl { get; set; }
}
