package verify

import (
	"encoding/json"
	"io"
)

// FormatJSON writes the VerifyResult as compact JSON to the given writer.
// Returns an error if JSON encoding fails.
func FormatJSON(w io.Writer, result *VerifyResult) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(result)
}
