using Pulumi;

class MyStack : Stack
{
    public MyStack()
    {
        this.ImageName = "pulumi/pulumi:latest";
    }

    [Output("imageName")]
    public Output<string> ImageName { get; set; }
}
