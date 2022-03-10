package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		ctx.Export("referencedImageName", placeholder_org_namEstackreferenceproducerPLACEHOLDER_STACK_NAME.Outputs.ImageName)
		return nil
	})
}
