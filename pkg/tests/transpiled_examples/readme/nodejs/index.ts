import * as pulumi from "@pulumi/pulumi";
import * from "fs";

export const strVar = "foo";
export const arrVar = [
    "fizz",
    "buzz",
];
export const readme = fs.readFileSync("./Pulumi.README.md");
