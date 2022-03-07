# Results of testing Programgen against examples

## Methodology

- All examples were generated with `pcl.AllowMissingProperties`,
  `pcl.AllowMissingVariables` and `pcl.SkipResourceTypechecking` set.
- There was no effort to type-check the resulting YAML, beyond generating
  without errors.
- All examples are classic providers because they all leverage
  [pulumi-terraform-bridge](https://github.com/pulumi/pulumi-terraform-bridge),
  which is where I inserted the code to extract the example pcl. 
- Because multiple errors can prevent a example from outputting valid PCL, it is
  possible that there are more causes of errors then failed tests. This is ok.

## Results

### For aws-clasic
50 Errors out of 795 examples  
45 failures are due to missing functions:  
    Fn::element. was needed by 1 examples.  
    Fn::fileArchive. was needed by 3 examples.  
    Fn::filebase64. was needed by 5 examples.  
    Fn::filebase64sha256. was needed by 2 examples.  
    Fn::readFile. was needed by 39 examples.  
    Fn::sha1. was needed by 7 examples.  
3 failures are due to missing expressions:  
    *model.BinaryOpExpression; was needed by 2 examples.  
    *model.UnaryOpExpression; was needed by 1 examples.  
Splat Expression needed for 2 examples.  
For Expression needed for 1 examples.  
panic: fatal: A failure has occurred: Non-inline expressions are not implemented yet  

### For gcp
21 Errors out of 434 examples  
18 failures are due to missing functions:  
    Fn::filebase64. was needed by 7 examples.  
    Fn::readFile. was needed by 19 examples.  
0 failures are due to missing expressions:  
Splat Expression needed for 1 examples.  
For Expression needed for 0 examples.  

### For azure-classic
125 Errors out of 930 examples  
20 failures are due to missing functions:  
    Fn::filebase64. was needed by 8 examples.  
    Fn::readFile. was needed by 12 examples.  
19 failures are due to missing expressions:  
    *model.BinaryOpExpression; was needed by 18 examples.  
    *model.IndexExpression; was needed by 1 examples.  
Splat Expression needed for 1 examples.  
For Expression needed for 0 examples.  

Note: 96 errors were unknown functions or invokes.

## Conclusion
`Fn::readFile` is needed for 70 examples. `Fn::fileBase` is needed by 20
examples. `Fn::sha` is needed by 7 examples. No other function is needed by 5
examples. To facilitate examples, we should implement `Fn::readFile`.
