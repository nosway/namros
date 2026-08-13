package mcpops

import "fmt"

type UnknownResourceError struct {
	URI string
}

func (e UnknownResourceError) Error() string {
	return fmt.Sprintf("unknown MCP resource %q", e.URI)
}

func ErrUnknownResource(uri string) error {
	return UnknownResourceError{URI: uri}
}

type UnknownToolError struct {
	Name string
}

func (e UnknownToolError) Error() string {
	return fmt.Sprintf("unknown MCP tool %q", e.Name)
}

func ErrUnknownTool(name string) error {
	return UnknownToolError{Name: name}
}
