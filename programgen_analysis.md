# Results of testing Programgen against examples

## Methodology

- All examples were generated with `pcl.AllowMissingProperties`,
  `pcl.AllowMissingVariables` and `pcl.SkipResourceTypechecking` set.
- There was no effort to type-check the resulting YAML, beyond generating
  without errors.
- All examples are classic providers because they all leverage
  [pulumi-terraform-bridge](https://github.com/pulumi/pulumi-terraform-bridge),
  which is where I inserted the code to extract the example pcl. 

## Results

### For aws-clasic
51 Errors out of 795 examples
40 failures are due to missing functions:
    Fn::element was needed by 1 examples.
    Fn::fileArchive was needed by 3 examples.
    Fn::filebase was needed by 4 examples.
    Fn::readFile was needed by 25 examples.
    Fn::sha was needed by 7 examples.

### For gcp
21 Errors out of 434 examples
13 failures are due to missing functions:
    Fn::filebase was needed by 1 examples.
    Fn::readFile was needed by 12 examples.

### For azure-classic
125 Errors out of 930 examples
18 failures are due to missing functions:
    Fn::filebase was needed by 6 examples.
    Fn::readFile was needed by 12 examples.

Note: 96 errors were unknown functions or invokes.

## Conclusion
`Fn::readFile` is needed for 49 examples. `Fn::fileBase` is needed by 11
examples. `Fn::sha` is needed by 7 examples. No other function is needed by 5
examples.
