// Package out prints the compact JSON contract that the agent reads on stdout.
package out

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Error is a failure the agent can act on: a stable code, a hint and the next command to run.
type Error struct {
	Code          string `json:"code"`
	Hint          string `json:"hint"`
	NextStep      string `json:"next_step"`
	RetryAfterSec int    `json:"retry_after_s,omitempty"`
	Details       any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Hint }

// Errf builds an *Error.
func Errf(code, next string, format string, a ...any) *Error {
	return &Error{Code: code, Hint: fmt.Sprintf(format, a...), NextStep: next}
}

// Print writes v as one line of JSON to stdout.
func Print(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// Fail prints err in the error contract and exits with a non-zero code.
func Fail(err error) {
	var e *Error
	if !errors.As(err, &e) {
		e = &Error{Code: "INTERNAL", Hint: err.Error(), NextStep: "Report this error to the user. Do not try to compute the analysis yourself."}
	}
	Print(struct {
		Status string `json:"status"`
		*Error
	}{"error", e})
	os.Exit(1)
}

// WriteJSON writes v as indented JSON to path.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// ReadJSON reads JSON from path into v.
func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
