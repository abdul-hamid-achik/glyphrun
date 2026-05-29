package spec

import (
	"os"

	"gopkg.in/yaml.v3"
)

func StampContractHash(path string, hash string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	if len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		return os.ErrInvalid
	}
	setMappingScalar(node.Content[0], "contractHash", hash)
	out, err := yaml.Marshal(&node)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func setMappingScalar(node *yaml.Node, key string, value string) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}
