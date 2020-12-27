package pulumiformation

// Template is a CloudFormation-compatible template structure which defines a Pulumi stack
// containing cloud infrastructure resources.
type Template struct {
	// Description is an informational bit of metadata about this template.
	Description string `json:",omitempty"`
	// Mappings provides the ability to have a static set of maps for programs that need to
	// perform lookups using Fn::FindInMap. For instance, we can map from region name to AMI IDs:
	//      "Mappings": {
	//          "RegionMap": {
	//              "us-east-1"     : { "HVM64": "ami-0ff8a91507f77f867" },
	//              "us-west-1"     : { "HVM64": "ami-0bdb828fd58c52235" },
	//              "eu-west-1"     : { "HVM64": "ami-047bb4163c506cd98" },
	//              "ap-southeast-1": { "HVM64": "ami-08569b978cc4dfa10" },
	//              "ap-northeast-1": { "HVM64": "ami-06cd52961ce9f0d85" }
	//          }
	//      }
	// Read more at https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/mappings-section-structure.html.
	Mappings map[string]map[string]map[string]string `json:",omitempty"`
	// Parameters allows the template to be conditional based on Pulumi configuration values. Read more at
	// https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/parameters-section-structure.html.
	Parameters map[string]*Parameter `json:",omitempty"`
	// Conditions can optionally contain a set of statements that defines the circumstances under which
	// entities are created or configured. This can be based on parameters to enable dynamic resource creation.
	// Read more at https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/conditions-section-structure.html.
	Conditions map[string]interface{} `json:",omitempty"`
	// Resources is a required section that declares resources you want to include in the stack.
	// Read more at https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html.
	Resources map[string]*Resource `json:",omitempty"`
	// Outputs declares a set of output values that will be exported from the stack and usable from other stacks.
	// Read more at https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/outputs-section-structure.html.
	Outputs map[string]*Output `json:",omitempty"`
}

// Parameter represents a single configurable parameter for this template. The parameters are
// validated before evaluating the template and may be specified using the Pulumi configuration system.
// Read more at http://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/parameters-section-structure.html.
type Parameter struct {
	// Type is the (required) data type for the parameter. It can be one of: `String`, `Number`,
	// `List<Number>`, or `CommaDelimetedList`.
	Type string
	// AllowedPattern is a regular expression that represents the patterns to allow for string types.
	AllowedPattern *string `json:",omitempty"`
	// AllowedValues is an array containing the list of values allowed for the parameter.
	AllowedValues *[]string `json:",omitempty"`
	// ConstraintDescription is a string that explains a constraint when the constraint is violated.
	ConstraintDescription string `json:",omitempty"`
	// Default is a value of the appropriate type for the template to use if no value is specified.
	Default interface{} `json:",omitempty"`
	// Description is a string that describes the parameter.
	Description string `json:",omitempty"`
	// NoEcho masks the parameter by marking it a secret.
	NoEcho *bool `json:",omitempty"`
	// MaxLength is an integer value that determines the largest number of characters you want to allow for strings.
	MaxLength *int64 `json:",omitempty"`
	// MaxValue is a numeric value that determines the largest numeric value you want to allow for numbers.
	MaxValue *int64 `json:",omitempty"`
	// MinLength is an integer value that determines the smallest number of characters you want to allow for strings.
	MinLength *int64 `json:",omitempty"`
	// MinValue is a numeric value that determines the smallest numeric value you want to allow for numbers.
	MinValue *int64 `json:",omitempty"`
}

// Resource declares a single infrastructure resource, such as an AWS S3 bucket or EC2 instance,
// complete with its properties and various other behavioral elements. Read more at
// https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html.
type Resource struct {
	// Type is the Pulumi type token for this resource.
	Type string
	// Condition makes this resource's creation conditional upon a predefined Condition attribute;
	// see https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/conditions-section-structure.html.
	Condition string `json:",omitempty"`
	// DependsOn makes this resource explicitly depend on another resource, by name, so that it won't
	// be created before the dependent finishes being created (and the reverse for destruction). Normally,
	// Pulumi automatically tracks implicit dependencies through inputs/outputs, but this can be used when
	// dependencies aren't captured purely from input/output edges.
	DependsOn []string `json:",omitempty"`
	// Metadata enables arbitrary metadata values to be associated with a resource.
	Metadata map[string]interface{} `json:",omitempty"`
	// Properties contains the primary resource-specific keys and values to initialize the resource state.
	Properties map[string]interface{} `json:",omitempty"`
}

// Output represents a single template output directive, which manifest as Pulumi exports. Read more
// at http://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/outputs-section-structure.html.
type Output struct {
	// Value is the (required) value of the property. The value of an output can include literals,
	// references, mapping values, builtin functions, and so on.
	Value interface{}
	// Description is an optional string that describes the output.
	Description string `json:",omitempty"`
}

// Note that there are many AWS-specific properties not supported by the above:
//     * Parameter: we don't support AWS-specific or SSM parameters types.
//     * Resource: there are several advanced policies -- CreationPolicy, DeletionPolicy, UpdatePolicy,
//       and UpdateReplacePolicy. These are largely about AWS autoscaling groups. For more information, see
//       https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-product-attribute-reference.html
//     * Output: we don't support cross-stack outputs that are different than ordinary stack outputs.
//       See https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/resources-section-structure.html.
