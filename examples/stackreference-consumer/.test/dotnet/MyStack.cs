using Pulumi;

class MyStack : Stack
{
    public MyStack()
    {
        this.ReferencedImageName = placeholder_org_namEstackreferenceproducerPLACEHOLDER_STACK_NAME.Outputs.ImageName;
    }

    [Output("referencedImageName")]
    public Output<string> ReferencedImageName { get; set; }
}
