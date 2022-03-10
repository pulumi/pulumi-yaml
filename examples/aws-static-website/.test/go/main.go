package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws-native/sdk/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v4/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		sitebucket, err := s3.NewBucket(ctx, "sitebucket", &s3.BucketArgs{
			WebsiteConfiguration: &s3.BucketWebsiteConfigurationArgs{
				IndexDocument: pulumi.String("index.html"),
			},
		})
		if err != nil {
			return err
		}
		_, err = s3.NewBucketObject(ctx, "indexhtml", &s3.BucketObjectArgs{
			Bucket:      sitebucket.ID(),
			Source:      pulumi.NewFileAsset("./www/index.html"),
			Acl:         pulumi.String("public-read"),
			ContentType: pulumi.String("text/html"),
		})
		if err != nil {
			return err
		}
		_, err = s3.NewBucketObject(ctx, "faviconpng", &s3.BucketObjectArgs{
			Bucket:      sitebucket.ID(),
			Source:      pulumi.NewFileAsset("./www/favicon.png"),
			Acl:         pulumi.String("public-read"),
			ContentType: pulumi.String("image/png"),
		})
		if err != nil {
			return err
		}
		_, err = s3.NewBucketPolicy(ctx, "bucketPolicy", &s3.BucketPolicyArgs{
			Bucket: sitebucket.ID(),
			Policy: sitebucket.Arn.ApplyT(func(arn string) (string, error) {
				return fmt.Sprintf("%v%v%v", "{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": \"*\",\n      \"Action\": [\"s3:GetObject\"],\n      \"Resource\": [\"", arn, "/*\"]\n    }\n  ]\n}\n"), nil
			}).(pulumi.StringOutput),
		})
		if err != nil {
			return err
		}
		ctx.Export("bucketName", sitebucket.BucketName)
		ctx.Export("websiteUrl", sitebucket.WebsiteURL)
		return nil
	})
}
