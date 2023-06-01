package linter

import "fmt"

// reference represents a reference to a Yale secret in a Deployment or StatefulSet manifest
type reference struct {
	// filename is the name of the file containing the reference
	filename string
	// lineno is the line number of the reference
	lineno int
	// kind is the kind of resource containing the reference
	kind string
	// name is the name of the resource containing the reference
	name string
	// secret is the name of the referenced Yale secret
	secret string
}

// summarize returns a human-readable summary of the reference
func (r reference) summarize() string {
	return fmt.Sprintf("%s:%d -- %s %s references Yale secret %s", r.filename, r.lineno, r.kind, r.name, r.secret)
}
