package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ICEServer is one STUN/TURN server offered to WebRTC clients. It mirrors the
// browser RTCIceServer shape, except that `urls` may be given as either a
// single string or an array in configuration, and is always normalised to a
// slice on the way out.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// UnmarshalJSON accepts `urls` as a string or an array of strings.
func (s *ICEServer) UnmarshalJSON(data []byte) error {
	var raw struct {
		URLs       json.RawMessage `json:"urls"`
		URL        string          `json:"url"`
		Username   string          `json:"username"`
		Credential string          `json:"credential"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	urls, err := decodeURLList(raw.URLs)
	if err != nil {
		return err
	}
	if len(urls) == 0 && strings.TrimSpace(raw.URL) != "" {
		urls = []string{strings.TrimSpace(raw.URL)}
	}
	s.URLs = urls
	s.Username = raw.Username
	s.Credential = raw.Credential
	return nil
}

// decodeURLList reads a JSON value that is either a string or an array of
// strings into a slice.
func decodeURLList(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if trimmed := strings.TrimSpace(single); trimmed != "" {
			return []string{trimmed}, nil
		}
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("urls must be a string or an array of strings: %w", err)
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out, nil
}

// parseICEServers decodes the ICE_SERVERS env value. An empty value yields a nil
// slice, meaning clients fall back to host candidates only (fine on localhost).
func parseICEServers(raw string) ([]ICEServer, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var servers []ICEServer
	if err := json.Unmarshal([]byte(trimmed), &servers); err != nil {
		return nil, fmt.Errorf("must be a JSON array of {urls,...} objects: %w", err)
	}
	return servers, nil
}
