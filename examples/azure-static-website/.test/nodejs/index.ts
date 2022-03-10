import * as pulumi from "@pulumi/pulumi";
import * as azure_native from "@pulumi/azure-native";

const staticsitegroup = new azure_native.resources.ResourceGroup("staticsitegroup", {});
const storageaccount = new azure_native.storage.StorageAccount("storageaccount", {
    resourceGroupName: staticsitegroup.name,
    kind: "StorageV2",
    sku: {
        name: "Standard_LRS",
    },
});
const staticwebsite = new azure_native.storage.StorageAccountStaticWebsite("staticwebsite", {
    resourceGroupName: staticsitegroup.name,
    accountName: storageaccount.name,
    indexDocument: "index.html",
    error404Document: "404.html",
});
const indexhtml = new azure_native.storage.Blob("indexhtml", {
    resourceGroupName: staticsitegroup.name,
    accountName: storageaccount.name,
    containerName: staticwebsite.containerName,
    contentType: "text/html",
    type: "Block",
    source: new pulumi.asset.FileAsset("./www/index.html"),
});
const faviconpng = new azure_native.storage.Blob("faviconpng", {
    resourceGroupName: staticsitegroup.name,
    accountName: storageaccount.name,
    containerName: staticwebsite.containerName,
    contentType: "image/png",
    type: "Block",
    source: new pulumi.asset.FileAsset("./www/favicon.png"),
});
const _404html = new azure_native.storage.Blob("_404html", {
    resourceGroupName: staticsitegroup.name,
    accountName: storageaccount.name,
    containerName: staticwebsite.containerName,
    contentType: "text/html",
    type: "Block",
    source: new pulumi.asset.FileAsset("./www/404.html"),
});
export const endpoint = storageaccount.primaryEndpoints.apply(primaryEndpoints => primaryEndpoints.web);
