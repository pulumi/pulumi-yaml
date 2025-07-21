package main

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		myValue := cfg.Require("myValue")
		computedValue := fmt.Sprintf("%v-suffix", myValue)
		ctx.Export("result", pulumi.StringMap{
			"value": computedValue,
		})
		return nil
	})
}
