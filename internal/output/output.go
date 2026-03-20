package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// Format is the output format type
type Format string

const (
	FormatPretty Format = "pretty"
	FormatJSON   Format = "json"
)

// CurrentFormat holds the global output format
var CurrentFormat Format = FormatPretty

// Result is a generic output result that can be rendered as JSON or pretty
type Result struct {
	Data    interface{}
	Pretty  func()
}

// Print outputs the result in the current format
func Print(r Result) {
	if CurrentFormat == FormatJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(r.Data)
	} else {
		if r.Pretty != nil {
			r.Pretty()
		} else {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(r.Data)
		}
	}
}

// Fail prints an error and exits
func Fail(message string) {
	if CurrentFormat == FormatJSON {
		data := map[string]string{"error": message}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(data)
	} else {
		fmt.Fprintf(os.Stderr, "\n  Error: %s\n\n", message)
	}
	os.Exit(1)
}
