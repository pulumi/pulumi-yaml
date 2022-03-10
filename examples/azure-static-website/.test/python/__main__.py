import pulumi
import pulumi_azure_native as azure_native

staticsitegroup = azure_native.resources.ResourceGroup("staticsitegroup")
storageaccount = azure_native.storage.StorageAccount("storageaccount",
    resource_group_name=staticsitegroup.name,
    kind="StorageV2",
    sku=azure_native.storage.SkuArgs(
        %!v(PANIC=Format method: interface conversion: model.Expression is *model.TemplateExpression, not *model.LiteralValueExpression))
    staticwebsite = azure_native.storage.StorageAccountStaticWebsite("staticwebsite",
        resource_group_name=staticsitegroup.name,
        account_name=storageaccount.name,
        index_document="index.html",
        error404_document="404.html")
    indexhtml = azure_native.storage.Blob("indexhtml",
        resource_group_name=staticsitegroup.name,
        account_name=storageaccount.name,
        container_name=staticwebsite.container_name,
        content_type="text/html",
        type="Block",
        source=pulumi.FileAsset("./www/index.html"))
    faviconpng = azure_native.storage.Blob("faviconpng",
        resource_group_name=staticsitegroup.name,
        account_name=storageaccount.name,
        container_name=staticwebsite.container_name,
        content_type="image/png",
        type="Block",
        source=pulumi.FileAsset("./www/favicon.png"))
    _404html = azure_native.storage.Blob("_404html",
        resource_group_name=staticsitegroup.name,
        account_name=storageaccount.name,
        container_name=staticwebsite.container_name,
        content_type="text/html",
        type="Block",
        source=pulumi.FileAsset("./www/404.html"))
    pulumi.export("endpoint", storageaccount.primary_endpoints.web)
