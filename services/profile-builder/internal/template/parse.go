package template

import "gopkg.in/yaml.v3"

// Parse decodes a YAML profile template.
func Parse(data []byte) (*Profile, error) {
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
