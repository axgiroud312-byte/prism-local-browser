package cookies

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var syntheticCookieTime = time.Unix(1800000000, 0)

func oneSyntheticCookie(t *testing.T, text string) Cookie {
	t.Helper()
	parsed := Parse(text, syntheticCookieTime)
	if len(parsed.Values) != 1 || len(parsed.Rows) != 1 || parsed.Rows[0].ErrorCode != "" {
		t.Fatal("synthetic valid Cookie was rejected")
	}
	return parsed.Values[parsed.Rows[0].Index]
}

func TestJSONEmptyValueAndSessionKeepOmittedExpiryAndSameSite(t *testing.T) {
	value := oneSyntheticCookie(t, `[{"name":"synthetic-empty","value":"","domain":"example.test","path":"/","secure":false,"httpOnly":true,"session":true}]`)
	encoded, _ := json.Marshal(value)
	if value.Value != "" || value.Expires != nil || value.SameSite != "" || !strings.Contains(string(encoded), `"value":""`) || strings.Contains(string(encoded), `"expires"`) || strings.Contains(string(encoded), `"url"`) {
		t.Fatal("empty/session/default attributes were rewritten in CDP payload")
	}
}

func TestJSONEpochIsExpiredButNetscapeZeroIsSession(t *testing.T) {
	jsonValue := Parse(`[{"name":"synthetic-epoch","value":"x","domain":"example.test","expires":0}]`, syntheticCookieTime)
	netscape := Parse("example.test\tFALSE\t/\tFALSE\t0\tsynthetic-session\t\n", syntheticCookieTime)
	if !jsonValue.Rows[0].Expired || jsonValue.Rows[0].Session || !netscape.Rows[0].Session || netscape.Rows[0].Expired || netscape.Values[1].Value != "" {
		t.Fatal("Netscape session sentinel and CDP epoch were conflated")
	}
}

func TestCookieExpiryConflictAndUnsupportedFieldsAreNotSilentlyDiscarded(t *testing.T) {
	for _, fields := range []string{`"session":true,"expires":1800000010`, `"session":false`, `"expires":1800000010,"expirationDate":1800000020`, `"expires":1800000000000`, `"partitionKeyOpaque":true`, `"storeId":"0"`, `"secure":null`, `"name":"duplicate"`} {
		parsed := Parse(`[{"name":"synthetic-invalid","value":"x","domain":"example.test",`+fields+`}]`, syntheticCookieTime)
		if len(parsed.Values) != 0 || parsed.Rows[0].ErrorCode == "" {
			t.Fatal("unsupported/conflicting metadata was silently rewritten")
		}
	}
}

func TestNetscapeHttpOnlyAndSubdomainFlagRetainCookieScope(t *testing.T) {
	domain := oneSyntheticCookie(t, "#HttpOnly_example.test\tTRUE\t/account\tTRUE\t1800000050\tsynthetic\tx")
	host := oneSyntheticCookie(t, ".example.test\tFALSE\t/\tFALSE\t0\tsynthetic\tx")
	if !domain.HTTPOnly || !domain.Secure || domain.Domain != ".example.test" || domain.Path != "/account" || host.Domain != "example.test" || host.Expires != nil {
		t.Fatal("Netscape flag was interpreted as hostOnly rather than include-subdomains")
	}
}

func TestRequestHeaderCookieIsNotACompleteImport(t *testing.T) {
	parsed := Parse("synthetic-name=x; other=", syntheticCookieTime)
	if len(parsed.Values) != 0 || parsed.Rows[0].ErrorCode != "COOKIE_FORMAT_INCOMPLETE" {
		t.Fatal("request header without domain/path was accepted as full Cookie")
	}
}

func TestPartitionKeyFalseIsRequiredPreservedAndNotDowngraded(t *testing.T) {
	value := oneSyntheticCookie(t, `[{"name":"synthetic-chips","value":"x","domain":"example.test","secure":true,"partitionKey":{"topLevelSite":"https://example.test","hasCrossSiteAncestor":false}}]`)
	encoded, _ := json.Marshal(value)
	if value.PartitionKey == nil || !strings.Contains(string(encoded), `"hasCrossSiteAncestor":false`) {
		t.Fatal("partition false was lost to omitempty")
	}
	for _, key := range []string{`"https://example.test"`, `{"topLevelSite":"https://example.test"}`, `{"topLevelSite":"","hasCrossSiteAncestor":false}`, `{"topLevelSite":"https://a.example.test","hasCrossSiteAncestor":true}`, `{"topLevelSite":"https://example.test/path","hasCrossSiteAncestor":false}`} {
		parsed := Parse(`[{"name":"synthetic-chips","value":"x","domain":"example.test","secure":true,"partitionKey":`+key+`}]`, syntheticCookieTime)
		if len(parsed.Values) != 0 {
			t.Fatal("unrepresentable partition key was removed or normalized without consent")
		}
	}
}

func TestSecurityPrefixesAndSameSiteNoneAreNotAutoRepaired(t *testing.T) {
	for _, fields := range []string{`"name":"__Secure-test","secure":false`, `"name":"__Host-test","secure":true,"domain":".example.test"`, `"name":"__Host-test","secure":true,"path":"/account"`, `"name":"synthetic","sameSite":"None","secure":false`} {
		// Build a complete unique-key object instead of a duplicate-name case.
		var data map[string]any
		_ = json.Unmarshal([]byte(`{"value":"x","domain":"example.test","path":"/"}`), &data)
		var extras map[string]any
		_ = json.Unmarshal([]byte("{"+fields+"}"), &extras)
		for key, value := range extras {
			data[key] = value
		}
		encoded, _ := json.Marshal([]any{data})
		parsed := Parse(string(encoded), syntheticCookieTime)
		if len(parsed.Values) != 0 || parsed.Rows[0].ErrorCode != "COOKIE_SECURITY_INVALID" {
			t.Fatal("security-invalid Cookie was weakened or silently corrected")
		}
	}
}

func TestCookieDuplicateIdentityIncludesPathDomainDotAndPartition(t *testing.T) {
	parsed := Parse(`[{"name":"synthetic","value":"1","domain":"example.test"},{"name":"synthetic","value":"2","domain":"example.test"},{"name":"synthetic","value":"3","domain":".example.test"},{"name":"synthetic","value":"4","domain":"example.test","path":"/other"},{"name":"synthetic","value":"5","domain":"example.test","secure":true,"partitionKey":{"topLevelSite":"https://example.test","hasCrossSiteAncestor":false}}]`, syntheticCookieTime)
	if len(parsed.Values) != 5 || !parsed.Rows[0].Conflict || !parsed.Rows[1].Conflict || parsed.Rows[2].Conflict || parsed.Rows[3].Conflict || parsed.Rows[4].Conflict {
		t.Fatal("unrelated scopes were merged or exact duplicates were missed")
	}
}

func TestCookieParserBoundsAndErrorsNeverEchoRawValue(t *testing.T) {
	marker := "SYNTHETIC_COOKIE_PRIVATE_VALUE"
	for _, text := range []string{`[{"name":"synthetic","value":"` + marker + `","domain":"bad/domain"}]`, strings.Repeat("x", MaxImportBytes+1), string([]byte{0xff})} {
		parsed := Parse(text, syntheticCookieTime)
		safe, _ := json.Marshal(parsed.Rows)
		if len(parsed.Values) != 0 || strings.Contains(string(safe), marker) {
			t.Fatal("invalid input entered safe preview or echoed the Cookie value")
		}
	}
}

func TestExactReadbackMustMatchValueAndAllDeclaredCookieSemantics(t *testing.T) {
	expected := oneSyntheticCookie(t, `[{"name":"synthetic","value":"x","domain":".example.test","httpOnly":true,"sameSite":"Strict","expires":1800000010.123456,"priority":"High","sourceScheme":"Secure","sourcePort":443}]`)
	stored := Stored{Cookie: expected, Session: false}
	if !Matches(expected, []Stored{stored}) {
		t.Fatal("exact full readback was rejected")
	}
	for _, change := range []func(*Stored){func(value *Stored) { value.Value = "other" }, func(value *Stored) { value.Domain = "example.test" }, func(value *Stored) { value.Path = "/other" }, func(value *Stored) { value.HTTPOnly = false }, func(value *Stored) { value.SameSite = "Lax" }, func(value *Stored) { expiry := *value.Expires - 100; value.Expires = &expiry }, func(value *Stored) { value.Session = true }, func(value *Stored) { opaque := true; value.PartitionKeyOpaque = &opaque }, func(value *Stored) { value.Priority = "Medium" }, func(value *Stored) { value.SourcePort = nil }} {
		candidate := stored
		change(&candidate)
		if Matches(expected, []Stored{candidate}) {
			t.Fatal("different Cookie was counted as verified")
		}
	}
	if Matches(expected, []Stored{stored, stored}) {
		t.Fatal("ambiguous readback took the first match")
	}
}

func TestScopeAndPathInputsCannotCanonicalizeIntoDifferentCookieKeys(t *testing.T) {
	for _, input := range []string{`[{"name":"synthetic","value":"x","domain":"..example.test","hostOnly":true}]`, "..example.test\tFALSE\t/\tFALSE\t0\tsynthetic\tx", `[{"name":"synthetic","value":"x","domain":".github.io"}]`, `[{"name":"synthetic","value":"x","domain":".localhost"}]`, `[{"name":"synthetic","value":"x","domain":"127.1"}]`, `[{"name":"synthetic","value":"x","domain":"0x7f000001"}]`} {
		parsed := Parse(input, syntheticCookieTime)
		if len(parsed.Values) != 0 {
			t.Fatal("explicit scope or numeric host could be changed to another write key")
		}
	}
	for _, path := range []string{"/a/../", "/./", "/%2e%2e/", "/a b", "/a;", "/a?b", "/a#b", "/a\\b", "/中文"} {
		data, _ := json.Marshal([]Cookie{{Name: "synthetic", Value: "x", Domain: "example.test", Path: path}})
		parsed := Parse(string(data), syntheticCookieTime)
		if len(parsed.Values) != 0 || parsed.Rows[0].ErrorCode != "COOKIE_PATH_UNSUPPORTED" {
			t.Fatal("path that Chromium might rewrite was accepted and could overwrite another key")
		}
	}
	if value := oneSyntheticCookie(t, `[{"name":"synthetic","value":"x","domain":"127.0.0.1","path":"/account/user-1"}]`); value.Domain != "127.0.0.1" {
		t.Fatal("canonical IPv4 host-only Cookie was rejected or altered")
	}
}

func TestOpaqueFalsePresenceCannotMatchAnOrdinaryUnpartitionedCookie(t *testing.T) {
	expected := oneSyntheticCookie(t, `[{"name":"synthetic","value":"x","domain":"example.test"}]`)
	var stored Stored
	if json.Unmarshal([]byte(`{"name":"synthetic","value":"x","domain":"example.test","path":"/","session":true,"expires":-1,"partitionKeyOpaque":false}`), &stored) != nil {
		t.Fatal("synthetic readback invalid")
	}
	if stored.PartitionKeyOpaque == nil || *stored.PartitionKeyOpaque || stored.SerializableIdentity() || Matches(expected, []Stored{stored}) {
		t.Fatal("nonce partition falsely matched an ordinary Cookie because opaque=false")
	}
}

func TestUnsupportedIPv6AndNegativeFractionalExpiryAndSourceConflictArePreviewErrors(t *testing.T) {
	for _, fields := range []string{`"domain":"::1"`, `"domain":"[::1]"`, `"domain":"example.test","expires":-0.5`, `"domain":"example.test","secure":true,"sourceScheme":"NonSecure"`} {
		parsed := Parse(`[{"name":"synthetic","value":"x",`+fields+`}]`, syntheticCookieTime)
		if len(parsed.Values) != 0 || parsed.Rows[0].ErrorCode == "" {
			t.Fatal("known unrepresentable CDP input was shown as valid")
		}
	}
}

func TestSessionReadbackRequiresMinusOneAndSameSiteAbsence(t *testing.T) {
	expected := oneSyntheticCookie(t, `[{"name":"synthetic-session","value":"","domain":"example.test"}]`)
	minusOne := float64(-1)
	stored := Stored{Cookie: expected, Session: true}
	stored.Expires = &minusOne
	if !Matches(expected, []Stored{stored}) {
		t.Fatal("session readback was rejected")
	}
	stored.SameSite = "None"
	if Matches(expected, []Stored{stored}) {
		t.Fatal("unspecified SameSite was conflated with None")
	}
	stored.SameSite, stored.Expires = "", nil
	if Matches(expected, []Stored{stored}) {
		t.Fatal("missing readback expiry was treated as the session sentinel")
	}
}
