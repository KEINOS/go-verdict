package verdict

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var errWritingJSONOutput = errors.New("writing json report")

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	err := enc.Encode(r)
	if err != nil {
		return fmt.Errorf("%w: %w", errWritingJSONOutput, err)
	}

	return nil
}
