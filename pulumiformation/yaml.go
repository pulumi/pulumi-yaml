package pulumiformation

import (
	"bytes"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// TagProcessor processes custom tags during YAML parsing of a Template.
type TagProcessor struct {
	template *Template
}

// UnmarshalYAML unmarshals a YAML document that includes customer tags.
func (i *TagProcessor) UnmarshalYAML(value *yaml.Node) error {
	resolved, err := resolveTags(value)
	if err != nil {
		return err
	}
	return resolved.Decode(i.template)
}

func resolveTags(node *yaml.Node) (*yaml.Node, error) {
	// Recursively resolve tags on any nested sequence or mapping nodes
	if node.Kind == yaml.SequenceNode || node.Kind == yaml.MappingNode {
		var err error
		for i := range node.Content {
			node.Content[i], err = resolveTags(node.Content[i])
			if err != nil {
				return nil, err
			}
		}
	}
	// Then process tags on this node
	switch node.Tag {
	case "!Ref":
		if node.Kind != yaml.ScalarNode {
			return nil, errors.New("expected !Ref argument to be a string")
		}
		return replacementNode(map[string]interface{}{
			"Ref": node.Value,
		})
	case "!GetAtt":
		if node.Kind != yaml.ScalarNode {
			return nil, errors.New("expected !GetAtt argument to be a string")
		}
		parts := strings.Split(node.Value, ".")
		if len(parts) != 2 {
			return nil, errors.Errorf("expected !GetAtt argument to be 'resourceName.propertyName', got '%s'", node.Value)
		}
		return replacementNode(map[string]interface{}{
			"Fn::GetAtt": parts,
		})
	case "!Sub":
		if node.Kind == yaml.ScalarNode {
			return replacementNode(map[string]interface{}{
				"Fn::Sub": node.Value,
			})
		} else if node.Kind == yaml.SequenceNode {
			return replacementNode(map[string]interface{}{
				"Fn::Sub": node.Content,
			})
		}
		return nil, errors.New("expected !Sub argument to be a scalar or list")
	case "!Select":
		if node.Kind != yaml.SequenceNode {
			return nil, errors.New("expected !Select argument to be a list")
		}
		return replacementNode(map[string]interface{}{
			"Fn::Select": node.Content,
		})
	case "!Join":
		if node.Kind != yaml.SequenceNode {
			return nil, errors.New("expected !Join argument to be a list")
		}
		return replacementNode(map[string]interface{}{
			"Fn::Join": node.Content,
		})
	}
	// If it was not a tagged node just return it.
	return node, nil
}

func replacementNode(v map[string]interface{}) (*yaml.Node, error) {
	var buf bytes.Buffer
	err := yaml.NewEncoder(&buf).Encode(v)
	if err != nil {
		return nil, err
	}
	var ret yaml.Node
	err = yaml.NewDecoder(&buf).Decode(&ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}
