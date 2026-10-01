// Package cookies parses native import input and compares real readback.
// Values are private execution material, never safe UI/report fields.
package cookies

import (
	"encoding/json"
	"math"
	"strings"
)

type PartitionKey struct {
	TopLevelSite         string `json:"topLevelSite"`
	HasCrossSiteAncestor bool   `json:"hasCrossSiteAncestor"`
}

type Cookie struct {
	Name         string        `json:"name"`
	Value        string        `json:"value"`
	Domain       string        `json:"domain"`
	Path         string        `json:"path"`
	Secure       bool          `json:"secure"`
	HTTPOnly     bool          `json:"httpOnly"`
	SameSite     string        `json:"sameSite,omitempty"`
	Expires      *float64      `json:"expires,omitempty"`
	PartitionKey *PartitionKey `json:"partitionKey,omitempty"`
	Priority     string        `json:"priority,omitempty"`
	SourceScheme string        `json:"sourceScheme,omitempty"`
	SourcePort   *int          `json:"sourcePort,omitempty"`
}

// Stored is private CDP readback, never an application response.
type Stored struct {
	Cookie
	Session            bool  `json:"session"`
	PartitionKeyOpaque *bool `json:"partitionKeyOpaque,omitempty"`
}

type ApplyResult struct {
	Status string
}

// Error contains a safe classification, never a cookie value or CDP reply.
type Error struct {
	Code, Message string
	Retryable     bool
}

func (e *Error) Error() string { return e.Message }

type Row struct {
	Index            int           `json:"index"`
	Name             string        `json:"name,omitempty"`
	Domain           string        `json:"domain,omitempty"`
	HostOnly         bool          `json:"hostOnly"`
	Path             string        `json:"path,omitempty"`
	Session          bool          `json:"session"`
	Secure           bool          `json:"secure"`
	HTTPOnly         bool          `json:"httpOnly"`
	SameSite         string        `json:"sameSite,omitempty"`
	Expires          *float64      `json:"expires,omitempty"`
	PartitionKey     *PartitionKey `json:"partitionKey,omitempty"`
	Expired          bool          `json:"expired"`
	Conflict         bool          `json:"conflict"`
	ExistingConflict bool          `json:"existingConflict"`
	ErrorCode        string        `json:"errorCode,omitempty"`
	Message          string        `json:"message,omitempty"`
}

type Parsed struct {
	Format string
	Rows   []Row
	Values map[int]Cookie
}

func Key(value Cookie) string {
	// An encoded tuple avoids delimiter collisions in cookie names/paths.
	encoded, _ := json.Marshal(struct {
		Name, Domain, Path string
		Partition          *PartitionKey
	}{value.Name, value.Domain, value.Path, value.PartitionKey})
	return string(encoded)
}

func Equal(expected, observed Cookie, session bool) bool {
	if Key(expected) != Key(observed) || expected.Value != observed.Value || expected.Secure != observed.Secure || expected.HTTPOnly != observed.HTTPOnly || (expected.Expires == nil) != session {
		return false
	}
	if observed.SameSite != expected.SameSite || expected.Priority != "" && expected.Priority != observed.Priority || expected.SourceScheme != "" && expected.SourceScheme != observed.SourceScheme {
		return false
	}
	if expected.SourcePort != nil && (observed.SourcePort == nil || *expected.SourcePort != *observed.SourcePort) {
		return false
	}
	// Chromium readback is in seconds and may round sub-microsecond digits.
	if expected.Expires == nil {
		return observed.Expires != nil && *observed.Expires == -1
	}
	return observed.Expires != nil && math.Abs(*expected.Expires-*observed.Expires) < 0.000001
}

func Matches(expected Cookie, stored []Stored) bool {
	count, matched := 0, false
	for _, value := range stored {
		if value.SerializableIdentity() && Key(expected) == Key(value.Cookie) {
			count++
			matched = Equal(expected, value.Cookie, value.Session)
		}
	}
	return count == 1 && matched
}

// An unserializable nonce partition can report opaque=false. Presence, not
// its boolean value, is the fact that must exclude an ordinary-cookie match.
func (value Stored) SerializableIdentity() bool { return value.PartitionKeyOpaque == nil }

func RowFor(index int, value Cookie, now float64) Row {
	return Row{Index: index, Name: value.Name, Domain: value.Domain, HostOnly: !strings.HasPrefix(value.Domain, "."), Path: value.Path, Session: value.Expires == nil, Secure: value.Secure, HTTPOnly: value.HTTPOnly, SameSite: value.SameSite, Expires: value.Expires, PartitionKey: value.PartitionKey, Expired: value.Expires != nil && *value.Expires <= now}
}
