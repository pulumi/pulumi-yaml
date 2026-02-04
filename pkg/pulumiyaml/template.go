// Copyright 2022, Pulumi Corporation.  All rights reserved.

package pulumiyaml

// Template is a YAML or JSON structure which defines a Pulumi stack containing cloud infrastructure resources.
type Template struct {
	// Name is the project name
	Name string `json:",omitempty" yaml:",omitempty"`
	// Description is an informational bit of metadata about this template.
	Description string `json:",omitempty" yaml:",omitempty"`
	// Configuration allows the template to be conditional based on Pulumi configuration values.
	Configuration map[string]*Configuration `json:",omitempty" yaml:",omitempty"`
	// Config is the configuration from the project-level `config` block.
	Config map[string]interface{} `json:",omitempty" yaml:",omitempty"`
	// Variables declares variables that will be used in the template.
	Variables map[string]interface{} `json:",omitempty" yaml:",omitempty"`
	// Resources is a required section that declares resources you want to include in the stack.
	Resources map[string]*Resource `json:",omitempty" yaml:",omitempty"`
	// Outputs declares a set of output values that will be exported from the stack and usable from other stacks.
	Outputs map[string]interface{} `json:",omitempty" yaml:",omitempty"`

	// TODO: Mappings and Conditions

	// Mappings provides the ability to have a static set of maps for programs that need to
	// perform lookups using fn::FindInMap. For instance, we can map from region name to AMI IDs:
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
	// Either `Type` or `Default` is required.

	// Type is the  data type for the parameter. It can be one of: `String`, `Number`,
	// `List<Number>`, or `List<String>`.
	Type string `yaml:",omitempty"`
	// Default is a value of the appropriate type for the template to use if no value is specified.
	Default interface{} `json:",omitempty" yaml:",omitempty"`
	// Secret masks the parameter by marking it a secret.
	Secret bool `yaml:",omitempty"`
}

// AliasSpec describes an alias for a resource. It can be either a string URN or an object with specific fields.
type AliasSpec struct {
	// URN is a direct URN alias (mutually exclusive with other fields)
	URN string `json:",omitempty" yaml:",omitempty"`
	// Name is the previous name of the resource
	Name string `json:",omitempty" yaml:",omitempty"`
	// Type is the previous type of the resource
	Type string `json:",omitempty" yaml:",omitempty"`
	// Parent is the previous parent of the resource (mutually exclusive with NoParent).
	Parent string `json:",omitempty" yaml:",omitempty"`
	// NoParent indicates the resource previously had no parent (mutually exclusive with Parent)
	NoParent bool `json:",omitempty" yaml:",omitempty"`
	// Stack is the previous stack of the resource
	Stack string `json:",omitempty" yaml:",omitempty"`
	// Project is the previous project of the resource
	Project string `json:",omitempty" yaml:",omitempty"`
}

// ResourceOptions describes additional options common to all Pulumi resources.
type ResourceOptions struct {
	// AdditionalSecretOutputs specifies properties that must be encrypted as secrets
	AdditionalSecretOutputs []string `json:",omitempty" yaml:",omitempty"`
	// Aliases specifies names that this resource used to be have so that renaming or refactoring doesn't replace it
	Aliases []AliasSpec `json:",omitempty" yaml:"Aliases,omitempty"`
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
	// ReplaceOnChanges forces a resource to be replaced when the targeted property is changed.
	ReplaceOnChanges []string `json:",omitempty" yaml:",omitempty"`
	// If set, the resource will be replaced if one of the specified resources is replaced.
	ReplaceWith []string `json:",omitempty" yaml:",omitempty"`
	// If set, the provider's Delete method will not be called for this resource if the specified resource is being
	// deleted as well.
	DeletedWith string `json:",omitempty" yaml:",omitempty"`
}

// Resource declares a single infrastructure resource, such as an AWS S3 bucket or EC2 instance,
// complete with its properties and various other behavioral elements.
type Resource struct {
	// Type is the Pulumi type token for this resource.
	Type string `yaml:""`
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
