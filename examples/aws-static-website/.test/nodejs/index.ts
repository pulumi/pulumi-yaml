import * as pulumi from "@pulumi/pulumi";
import * as aws from "@pulumi/aws";
import * as aws_native from "@pulumi/aws-native";

const sitebucket = new aws_native.s3.Bucket("sitebucket", {websiteConfiguration: {
    indexDocument: "index.html",
}});
const indexhtml = new aws.s3.BucketObject("indexhtml", {
    bucket: sitebucket.id,
    source: new pulumi.asset.FileAsset("./www/index.html"),
    acl: "public-read",
    contentType: "text/html",
});
const faviconpng = new aws.s3.BucketObject("faviconpng", {
    bucket: sitebucket.id,
    source: new pulumi.asset.FileAsset("./www/favicon.png"),
    acl: "public-read",
    contentType: "image/png",
});
const bucketPolicy = new aws.s3.BucketPolicy("bucketPolicy", {
    bucket: sitebucket.id,
    policy: pulumi.interpolate`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": "*",
      "Action": ["s3:GetObject"],
      "Resource": ["${sitebucket.arn}/*"]
    }
  ]
}
`,
});
export const bucketName = sitebucket.bucketName;
export const websiteUrl = sitebucket.websiteURL;
