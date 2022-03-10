package main

import (
	resources "github.com/pulumi/pulumi-azure-native/sdk/go/azure/resources"
	storage "github.com/pulumi/pulumi-azure-native/sdk/go/azure/storage"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		staticsitegroup, err := resources.NewResourceGroup(ctx, "staticsitegroup", nil)
		if err != nil {
			return err
		}
		storageaccount, err := storage.NewStorageAccount(ctx, "storageaccount", &storage.StorageAccountArgs{
			ResourceGroupName: staticsitegroup.Name,
			Kind:              pulumi.String("StorageV2"),
			Sku: &storage.SkuArgs{
				Name: pulumi.String("Standard_LRS"),
			},
		})
		if err != nil {
			return err
		}
		staticwebsite, err := storage.NewStorageAccountStaticWebsite(ctx, "staticwebsite", &storage.StorageAccountStaticWebsiteArgs{
			ResourceGroupName: staticsitegroup.Name,
			AccountName:       storageaccount.Name,
			IndexDocument:     pulumi.String("index.html"),
			Error404Document:  pulumi.String("404.html"),
		})
		if err != nil {
			return err
		}
		_, err = storage.NewBlob(ctx, "indexhtml", &storage.BlobArgs{
			ResourceGroupName: staticsitegroup.Name,
			AccountName:       storageaccount.Name,
			ContainerName:     staticwebsite.ContainerName,
			ContentType:       pulumi.String("text/html"),
			Type:              "Block",
			Source:            pulumi.NewFileAsset("./www/index.html"),
		})
		if err != nil {
			return err
		}
		_, err = storage.NewBlob(ctx, "faviconpng", &storage.BlobArgs{
			ResourceGroupName: staticsitegroup.Name,
			AccountName:       storageaccount.Name,
			ContainerName:     staticwebsite.ContainerName,
			ContentType:       pulumi.String("image/png"),
			Type:              "Block",
			Source:            pulumi.NewFileAsset("./www/favicon.png"),
		})
		if err != nil {
			return err
		}
		_, err = storage.NewBlob(ctx, "_404html", &storage.BlobArgs{
			ResourceGroupName: staticsitegroup.Name,
			AccountName:       storageaccount.Name,
			ContainerName:     staticwebsite.ContainerName,
			ContentType:       pulumi.String("text/html"),
			Type:              "Block",
			Source:            pulumi.NewFileAsset("./www/404.html"),
		})
		if err != nil {
			return err
		}
		ctx.Export("endpoint", storageaccount.PrimaryEndpoints.ApplyT(func(primaryEndpoints storage.EndpointsResponse) (string, error) {
			return primaryEndpoints.Web, nil
		}).(pulumi.StringOutput))
		return nil
	})
}
