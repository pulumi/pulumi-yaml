// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

// Template is a YAML or JSON structure which defines a Pulumi stack containing cloud infrastructure resources.
type Template struct {
	// Description is an informational bit of metadata about this template.
	Description string `json:",omitempty" yaml:",omitempty"`
	// Configuration allows the template to be conditional based on Pulumi configuration values.
	Configuration map[string]*Configuration `json:",omitempty" yaml:",omitempty"`
	// Resources is a required section that declares resources you want to include in the stack.
	Resources map[string]*Resource `json:",omitempty" yaml:",omitempty"`
	// Outputs declares a set of output values that will be exported from the stack and usable from other stacks.
	Outputs map[string]interface{} `json:",omitempty" yaml:",omitempty"`
	// Variables declared to simplify the program.
	Variables map[string]interface{} `json:",omitempty" yaml:",omitempty"`

	// TODO: Mappings and Conditions

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
	Mappings map[string]map[string]map[string]string `json:",omitempty" yaml:",omitempty"`
	// Conditions can optionally contain a set of statements that defines the circumstances under which
	// entities are created or configured. This can be based on parameters to enable dynamic resource creation.
	// Read more at https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/conditions-section-structure.html.
	Conditions map[string]interface{} `json:",omitempty" yaml:",omitempty"`
}

// Configuration represents a single configurable parameter for this template. The parameters are
// validated before evaluating the template and may be specified using the Pulumi configuration system.
type Configuration struct {
	// Type is the (required) data type for the parameter. It can be one of: `String`, `Number`,
	// `List<Number>`, or `CommaDelimetedList`.
	Type string `yaml:""`
	// Default is a value of the appropriate type for the template to use if no value is specified.
	Default interface{} `json:",omitempty" yaml:",omitempty"`
	// Secret masks the parameter by marking it a secret.
	Secret *bool `json:",omitempty" yaml:",omitempty"`

	// TODO: AllowedPattern, AllowedValues, ConstraintDescription, Description, MaxLength, MaxValue, MinLength, MinValue

	// AllowedPattern is a regular expression that represents the patterns to allow for string types.
	AllowedPattern *string `json:",omitempty" yaml:",omitempty"`
	// AllowedValues is an array containing the list of values allowed for the parameter.
	AllowedValues *[]string `json:",omitempty" yaml:",omitempty"`
	// ConstraintDescription is a string that explains a constraint when the constraint is violated.
	ConstraintDescription string `json:",omitempty" yaml:",omitempty"`
	// Description is a string that describes the parameter.
	Description string `json:",omitempty" yaml:",omitempty"`
	// MaxLength is an integer value that determines the largest number of characters you want to allow for strings.
	MaxLength *int64 `json:",omitempty" yaml:",omitempty"`
	// MaxValue is a numeric value that determines the largest numeric value you want to allow for numbers.
	MaxValue *int64 `json:",omitempty" yaml:",omitempty"`
	// MinLength is an integer value that determines the smallest number of characters you want to allow for strings.
	MinLength *int64 `json:",omitempty" yaml:",omitempty"`
	// MinValue is a numeric value that determines the smallest numeric value you want to allow for numbers.
	MinValue *int64 `json:",omitempty" yaml:",omitempty"`
}

// ResourceOptions describes additional options common to all Pulumi resources.
type ResourceOptions struct {
	// AdditionalSecretOutputs specifies properties that must be encrypted as secrets
	AdditionalSecretOutputs []string `json:",omitempty" yaml:",omitempty"`
	// Aliases specifies names that this resource used to be have so that renaming or refactoring doesn’t replace it
	Aliases []string `json:",omitempty" yaml:"Aliases,omitempty"`
	// CustomTimeouts overrides the default retry/timeout behavior for resource provisioning
	CustomTimeouts *CustomTimeoutResourceOption `json:",omitempty" yaml:",omitempty"`
	// DeleteBeforeReplace  overrides the default create-before-delete behavior when replacing
	DeleteBeforeReplace bool `json:",omitempty" yaml:",omitempty"`
	// DependsOn makes this resource explicitly depend on another resource, by name, so that it won't
	// be created before the dependent finishes being created (and the reverse for destruction). Normally,
	// Pulumi automatically tracks implicit dependencies through inputs/outputs, but this can be used when
	// dependencies aren't captured purely from input/output edges.
	DependsOn []string `json:",omitempty" yaml:",omitempty"`
	// IgnoreChangs declares that changes to certain properties should be ignored during diffing
	IgnoreChanges []string `json:",omitempty" yaml:",omitempty"`
	// Import adopts an existing resource from your cloud account under the control of Pulumi
	Import string `json:",omitempty" yaml:",omitempty"`
	// Parent specifies a parent for the resource
	Parent string `json:",omitempty" yaml:",omitempty"`
	// Protect prevents accidental deletion of a resource
	Protect bool `json:",omitempty" yaml:",omitempty"`
	// Provider specifies an explicitly configured provider, instead of using the default global provider
	Provider string `json:",omitempty" yaml:",omitempty"`
	// Version specifies a provider plugin version that should be used when operating on a resource
	Version string `json:",omitempty" yaml:",omitempty"`
}

// Resource declares a single infrastructure resource, such as an AWS S3 bucket or EC2 instance,
// complete with its properties and various other behavioral elements.
type Resource struct {
	// Type is the Pulumi type token for this resource.
	Type string `yaml:""`
	// Component indicates this resources is a component
	Component bool `yaml:",omitempty"`
	// Properties contains the primary resource-specific keys and values to initialize the resource state.
	Properties map[string]interface{} `json:",omitempty" yaml:",omitempty"`
	// Options contains all Pulumi resource options used to register the resource.
	ResourceOptions *ResourceOptions `json:",omitempty" yaml:",omitempty"`

	// TODO: Condition, Metadata

	// Condition makes this resource's creation conditional upon a predefined Condition attribute;
	// see https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/conditions-section-structure.html.
	Condition string `json:",omitempty" yaml:",omitempty"`
	// Metadata enables arbitrary metadata values to be associated with a resource.
	Metadata map[string]interface{} `json:",omitempty" yaml:",omitempty"`
}

// CustomTimeoutResourceOption provides a set of custom timeouts for create, update, and delete operations on a
// resource. These timeouts are specified using a duration string like "5m" (5 minutes), "40s" (40 seconds), or
// "1d" (1 day). Supported duration units are "ns", "us" (or "µs"), "ms", "s", "m", and "h" (nanoseconds,
// microseconds, milliseconds, seconds, minutes, and hours, respectively).
type CustomTimeoutResourceOption struct {
	// Create is the custom timeout for create operations.
	Create string `json:",omitempty" yaml:",omitempty"`
	// Delete is the custom timeout for delete operations.
	Delete string `json:",omitempty" yaml:",omitempty"`
	// Update is the custom timeout for update operations.
	Update string `json:",omitempty" yaml:",omitempty"`
}
