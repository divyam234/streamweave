package credentialjson

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

func Decode(value string, target any) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("credential is empty")
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		return json.Unmarshal([]byte(value), target)
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, encoding := range encodings {
		data, err := encoding.DecodeString(value)
		if err == nil && json.Valid(data) {
			return json.Unmarshal(data, target)
		}
	}
	return errors.New("credential must be JSON or base64-encoded JSON")
}
