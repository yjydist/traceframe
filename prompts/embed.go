package prompts

import "embed"

//go:embed critic/v1/* discovery/v1/* specialist/v1/*
var files embed.FS

func DiscoveryV1() (string, []byte, error) {
	system, err := files.ReadFile("discovery/v1/system.md")
	if err != nil {
		return "", nil, err
	}
	schema, err := files.ReadFile("discovery/v1/response.schema.json")
	if err != nil {
		return "", nil, err
	}
	return string(system), schema, nil
}

func CriticV1() (string, []byte, error) {
	system, err := files.ReadFile("critic/v1/system.md")
	if err != nil {
		return "", nil, err
	}
	schema, err := files.ReadFile("critic/v1/response.schema.json")
	if err != nil {
		return "", nil, err
	}
	return string(system), schema, nil
}

func SpecialistV1() (string, []byte, error) {
	system, err := files.ReadFile("specialist/v1/system.md")
	if err != nil {
		return "", nil, err
	}
	schema, err := files.ReadFile("discovery/v1/response.schema.json")
	if err != nil {
		return "", nil, err
	}
	return string(system), schema, nil
}
