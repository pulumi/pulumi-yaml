import pulumi
import os

pulumi.export("cwd0", os.getcwd())
pulumi.export("stack0", pulumi.get_stack())
pulumi.export("project0", pulumi.get_project())
