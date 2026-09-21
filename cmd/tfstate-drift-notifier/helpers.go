package main

import (
	"encoding/json"
	"io"
	"os"
)

// exit is a var so tests can intercept it instead of terminating the process.
var exit = os.Exit

// jsonEncoder returns a function that writes v as indented JSON to w.
func jsonEncoder(w io.Writer) func(v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode
}
