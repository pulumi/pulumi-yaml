using Pulumi;
using AzureNative = Pulumi.AzureNative;

class MyStack : Stack
{
    public MyStack()
    {
        var staticsitegroup = new AzureNative.Resources.ResourceGroup("staticsitegroup", new AzureNative.Resources.ResourceGroupArgs
        {
        });
        var storageaccount = new AzureNative.Storage.StorageAccount("storageaccount", new AzureNative.Storage.StorageAccountArgs
        {
            ResourceGroupName = staticsitegroup.Name,
            Kind = "StorageV2",
            Sku = new AzureNative.Storage.Inputs.SkuArgs
            {
                %!v(PANIC=Format method: interface conversion: model.Expression is *model.TemplateExpression, not *model.LiteralValueExpression),
            });
            var staticwebsite = new AzureNative.Storage.StorageAccountStaticWebsite("staticwebsite", new AzureNative.Storage.StorageAccountStaticWebsiteArgs
            {
                ResourceGroupName = staticsitegroup.Name,
                AccountName = storageaccount.Name,
                IndexDocument = "index.html",
                Error404Document = "404.html",
            });
            var indexhtml = new AzureNative.Storage.Blob("indexhtml", new AzureNative.Storage.BlobArgs
            {
                ResourceGroupName = staticsitegroup.Name,
                AccountName = storageaccount.Name,
                ContainerName = staticwebsite.ContainerName,
                ContentType = "text/html",
                Type = "Block",
                Source = new FileAsset("./www/index.html"),
            });
            var faviconpng = new AzureNative.Storage.Blob("faviconpng", new AzureNative.Storage.BlobArgs
            {
                ResourceGroupName = staticsitegroup.Name,
                AccountName = storageaccount.Name,
                ContainerName = staticwebsite.ContainerName,
                ContentType = "image/png",
                Type = "Block",
                Source = new FileAsset("./www/favicon.png"),
            });
            var _404html = new AzureNative.Storage.Blob("_404html", new AzureNative.Storage.BlobArgs
            {
                ResourceGroupName = staticsitegroup.Name,
                AccountName = storageaccount.Name,
                ContainerName = staticwebsite.ContainerName,
                ContentType = "text/html",
                Type = "Block",
                Source = new FileAsset("./www/404.html"),
            });
            this.Endpoint = storageaccount.PrimaryEndpoints.Apply(primaryEndpoints => primaryEndpoints.Web);
        }

        [Output("endpoint")]
        public Output<string> Endpoint { get; set; }
}
