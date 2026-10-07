package cookies

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

const MaxImportBytes = 2 << 20

func Parse(text string, now time.Time) Parsed {
	result := Parsed{Rows: []Row{}, Values: map[int]Cookie{}}
	if len(text) > MaxImportBytes || !utf8.ValidString(text) {
		result.Rows = append(result.Rows, Row{Index: 1, ErrorCode: "COOKIE_INPUT_INVALID", Message: "输入需为UTF-8且不超过2MiB；未保存或写入。"})
		return result
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(text, "\ufeff"))
	if strings.HasPrefix(trimmed, "[") {
		result.Format = "json"
		var entries []json.RawMessage
		if json.Unmarshal([]byte(trimmed), &entries) != nil {
			result.Rows = append(result.Rows, invalidRow(1, "COOKIE_JSON_INVALID", "JSON需为完整Cookie对象数组，未回显原文。"))
			return result
		}
		for index, raw := range entries {
			value, code, message := parseJSONCookie(raw)
			if code != "" {
				result.Rows = append(result.Rows, invalidRow(index+1, code, message))
				continue
			}
			result.Values[index+1] = value
			result.Rows = append(result.Rows, RowFor(index+1, value, float64(now.UnixNano())/1e9))
		}
	} else {
		result.Format = "netscape"
		for index, line := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
			line = strings.TrimSuffix(line, "\r")
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#HttpOnly_") {
				continue
			}
			value, code, message := parseNetscapeCookie(line)
			if code != "" {
				result.Rows = append(result.Rows, invalidRow(index+1, code, message))
				continue
			}
			result.Values[index+1] = value
			result.Rows = append(result.Rows, RowFor(index+1, value, float64(now.UnixNano())/1e9))
		}
	}
	if len(result.Rows) == 0 {
		result.Rows = append(result.Rows, invalidRow(1, "COOKIE_INPUT_EMPTY", "未找到Cookie记录；需要JSON数组或7列Netscape文本。"))
	}
	counts := map[string]int{}
	for _, value := range result.Values {
		counts[Key(value)]++
	}
	for index := range result.Rows {
		if value, ok := result.Values[result.Rows[index].Index]; ok {
			result.Rows[index].Conflict = counts[Key(value)] > 1
		}
	}
	return result
}

func invalidRow(index int, code, message string) Row {
	return Row{Index: index, ErrorCode: code, Message: message}
}

// Reject duplicate JSON keys rather than silently using the last credential.
func object(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("not an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("invalid key")
		}
		if _, exists := fields[name]; exists {
			return nil, errors.New("duplicate field")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, errors.New("invalid field")
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return fields, nil
}

func parseJSONCookie(raw []byte) (Cookie, string, string) {
	value := Cookie{Path: "/"}
	fields, err := object(raw)
	if err != nil {
		return value, "COOKIE_JSON_INVALID", "Cookie需为字段唯一的JSON对象，不能重复字段或使用null。"
	}
	allowed := map[string]bool{"name": true, "value": true, "domain": true, "path": true, "secure": true, "httpOnly": true, "sameSite": true, "expires": true, "expirationDate": true, "session": true, "hostOnly": true, "partitionKey": true, "priority": true, "sourceScheme": true, "sourcePort": true}
	for field := range fields {
		if !allowed[field] {
			return value, "COOKIE_FIELD_UNSUPPORTED", "含当前导入不支持的字段；请显式调整输入，不会静默丢弃字段。"
		}
	}
	read := func(name string, destination any, required bool) bool {
		raw, found := fields[name]
		return !found && !required || found && string(raw) != "null" && json.Unmarshal(raw, destination) == nil
	}
	if !read("name", &value.Name, true) || !read("value", &value.Value, true) || !read("domain", &value.Domain, true) || !read("path", &value.Path, false) || !read("secure", &value.Secure, false) || !read("httpOnly", &value.HTTPOnly, false) || !read("sameSite", &value.SameSite, false) || !read("priority", &value.Priority, false) || !read("sourceScheme", &value.SourceScheme, false) {
		return value, "COOKIE_FIELD_INVALID", "需要字符串name/value/domain；可选字段类型必须正确，value允许空字符串。"
	}
	if raw, found := fields["sourcePort"]; found {
		var port int
		if string(raw) == "null" || json.Unmarshal(raw, &port) != nil || port < -1 || port > 65535 || port == 0 {
			return value, "COOKIE_FIELD_INVALID", "sourcePort需为-1或1–65535整数，未替换其语义。"
		}
		value.SourcePort = &port
	}
	var session, hostOnly bool
	if !read("session", &session, false) || !read("hostOnly", &hostOnly, false) {
		return value, "COOKIE_FIELD_INVALID", "session/hostOnly需为布尔值。"
	}
	for _, field := range []string{"expires", "expirationDate"} {
		if raw, found := fields[field]; found {
			var expiry float64
			if string(raw) == "null" || json.Unmarshal(raw, &expiry) != nil || math.IsNaN(expiry) || math.IsInf(expiry, 0) || expiry < 0 && expiry != -1 || expiry > 253402300799 {
				return value, "COOKIE_TIME_INVALID", "有效期需为Unix秒，-1代表session；不接受毫秒或超出范围的时间。"
			}
			if value.Expires != nil && *value.Expires != expiry {
				return value, "COOKIE_TIME_CONFLICT", "expires与expirationDate不一致，没有猜测或续期。"
			}
			value.Expires = &expiry
		}
	}
	if _, found := fields["session"]; found {
		if session && value.Expires != nil && *value.Expires != -1 || !session && (value.Expires == nil || *value.Expires == -1) {
			return value, "COOKIE_TIME_CONFLICT", "session标记与有效期冲突，未擅自添加或删除有效期。"
		}
	}
	if value.Expires != nil && *value.Expires == -1 {
		value.Expires = nil
	}
	if strings.HasPrefix(value.Domain, "..") {
		return value, "COOKIE_DOMAIN_INVALID", "域名不能有多个前导点，不能反转明确的hostOnly作用域。"
	}
	if _, found := fields["hostOnly"]; found {
		value.Domain = strings.TrimPrefix(value.Domain, ".")
		if !hostOnly {
			value.Domain = "." + value.Domain
		}
	}
	if raw, found := fields["partitionKey"]; found {
		partition, err := object(raw)
		if err != nil || len(partition) != 2 {
			return value, "COOKIE_PARTITION_UNSUPPORTED", "分区键需包含topLevelSite及hasCrossSiteAncestor；不支持字符串、opaque或缺字段分区。"
		}
		var key PartitionKey
		if json.Unmarshal(partition["topLevelSite"], &key.TopLevelSite) != nil || string(partition["topLevelSite"]) == "null" || json.Unmarshal(partition["hasCrossSiteAncestor"], &key.HasCrossSiteAncestor) != nil || string(partition["hasCrossSiteAncestor"]) == "null" {
			return value, "COOKIE_PARTITION_UNSUPPORTED", "分区站点/跨站祖先标记类型无效，没有去除分区后写入。"
		}
		site, err := normalizeSite(key.TopLevelSite)
		if err != nil {
			return value, "COOKIE_PARTITION_UNSUPPORTED", "分区站点需为规范HTTP(S)schemeful site，不接受任意URL或opaque分区。"
		}
		key.TopLevelSite = site
		value.PartitionKey = &key
	}
	return validateCookie(value)
}

func parseNetscapeCookie(line string) (Cookie, string, string) {
	value := Cookie{HTTPOnly: strings.HasPrefix(line, "#HttpOnly_")}
	fields := strings.Split(strings.TrimPrefix(line, "#HttpOnly_"), "\t")
	if len(fields) != 7 {
		return value, "COOKIE_FORMAT_INCOMPLETE", "Netscape需7个制表符分隔字段；请求头Cookie没有域名等信息，不能当完整导入。"
	}
	value.Domain, value.Path, value.Name, value.Value = fields[0], fields[2], fields[5], fields[6]
	boolean := func(text string) (bool, bool) {
		if strings.EqualFold(text, "TRUE") {
			return true, true
		}
		return false, strings.EqualFold(text, "FALSE")
	}
	subdomains, valid := boolean(fields[1])
	if !valid {
		return value, "COOKIE_FIELD_INVALID", "Netscape子域名标记需为TRUE/FALSE。"
	}
	value.Secure, valid = boolean(fields[3])
	if !valid {
		return value, "COOKIE_FIELD_INVALID", "Netscape secure标记需为TRUE/FALSE。"
	}
	if strings.HasPrefix(value.Domain, "..") {
		return value, "COOKIE_DOMAIN_INVALID", "Netscape域名不能有多个前导点，未扩大子域作用域。"
	}
	value.Domain = strings.TrimPrefix(value.Domain, ".")
	if subdomains {
		value.Domain = "." + value.Domain
	}
	expires, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil || expires < -1 || expires > 253402300799 {
		return value, "COOKIE_TIME_INVALID", "Netscape有效期需为Unix秒；0或-1表示session，不擅自补未来时间。"
	}
	if expires > 0 {
		seconds := float64(expires)
		value.Expires = &seconds
	}
	return validateCookie(value)
}

func safeField(value string, max int) bool {
	if !utf8.ValidString(value) || len(value) > max {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func normalizeDomain(domain string) (string, error) {
	domainCookie := strings.HasPrefix(domain, ".")
	host := strings.TrimPrefix(domain, ".")
	if host == "" || strings.HasSuffix(host, ".") || !safeField(host, 253) {
		return "", errors.New("invalid cookie host")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if domainCookie || ip.Zone() != "" {
			return "", errors.New("invalid IP scope")
		}
		return ip.String(), nil
	}
	host, err := idna.Lookup.ToASCII(host)
	if err != nil || len(host) > 253 {
		return "", errors.New("invalid domain")
	}
	host = strings.ToLower(host)
	// Chromium also recognizes legacy abbreviated/hex/octal IPv4 URL hosts.
	// Only the canonical netip IPv4 branch above is a supported Cookie key.
	last := host[strings.LastIndex(host, ".")+1:]
	numeric := last != ""
	for _, c := range last {
		if c < '0' || c > '9' {
			numeric = false
		}
	}
	if numeric || strings.HasPrefix(last, "0x") {
		return "", errors.New("noncanonical numeric URL host")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", errors.New("invalid label")
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", errors.New("invalid label")
			}
		}
	}
	if domainCookie {
		suffix, _ := publicsuffix.PublicSuffix(host)
		if suffix == host || !strings.Contains(host, ".") {
			return "", errors.New("public suffix")
		}
		return "." + host, nil
	}
	return host, nil
}

func normalizeSite(site string) (string, error) {
	u, err := url.Parse(site)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", errors.New("invalid partition site")
	}
	host, err := normalizeDomain(u.Hostname())
	if err != nil {
		return "", err
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is6() {
			host = "[" + host + "]"
		}
	} else if strings.Contains(host, ".") {
		domain, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err != nil || domain != host {
			return "", errors.New("not a registrable site")
		}
	}
	return u.Scheme + "://" + host, nil
}

func validateCookie(value Cookie) (Cookie, string, string) {
	if value.Name == "" || !safeField(value.Name, 4096) || !safeField(value.Value, 4096) || len(value.Name)+len(value.Value) > 4096 || strings.Contains(value.Value, ";") {
		return value, "COOKIE_VALUE_INVALID", "name/value超出Cookie范围或含控制/分隔字符；空字符串value合法。"
	}
	for _, c := range value.Name {
		if c > 126 || strings.ContainsRune("()<>@,;:\\\"/[]?={} \t", c) {
			return value, "COOKIE_NAME_INVALID", "Cookie名称需为有效token，不回显无效内容。"
		}
	}
	if ip, err := netip.ParseAddr(strings.Trim(strings.TrimPrefix(value.Domain, "."), "[]")); err == nil && ip.Is6() {
		return value, "COOKIE_IPV6_UNSUPPORTED", "本次导入暂不支持IPv6 Cookie域，CDP写入/读回形态尚未闭合；未显示有效后降级写入。"
	}
	domain, err := normalizeDomain(value.Domain)
	if err != nil {
		return value, "COOKIE_DOMAIN_INVALID", "Cookie域名/IP范围无效，不允许公共后缀域Cookie或带scope的IP。"
	}
	value.Domain = domain
	if !canonicalCookiePath(value.Path) {
		return value, "COOKIE_PATH_UNSUPPORTED", "path需是可原样保存的ASCII绝对路径；暂不支持点段、百分号、空白或URL分隔字符，避免内核规范化覆盖其他键。"
	}
	if value.SameSite != "" && value.SameSite != "Strict" && value.SameSite != "Lax" && value.SameSite != "None" || value.Priority != "" && value.Priority != "Low" && value.Priority != "Medium" && value.Priority != "High" || value.SourceScheme != "" && value.SourceScheme != "Unset" && value.SourceScheme != "NonSecure" && value.SourceScheme != "Secure" {
		return value, "COOKIE_FIELD_INVALID", "sameSite/priority/sourceScheme枚举不支持，未转换或丢字段。"
	}
	if value.Secure && value.SourceScheme == "NonSecure" || value.SameSite == "None" && !value.Secure || value.PartitionKey != nil && !value.Secure || strings.HasPrefix(value.Name, "__Secure-") && !value.Secure || strings.HasPrefix(value.Name, "__Host-") && (!value.Secure || value.Path != "/" || strings.HasPrefix(value.Domain, ".")) {
		return value, "COOKIE_SECURITY_INVALID", "SameSite=None、分区、安全前缀或secure/sourceScheme约束不满足；不关闭secure或降低作用域后重试。"
	}
	return value, "", ""
}

// Conservative supported subset of Chromium's URL path canonicalization.
// Never silently rewrite a key before or after overwriting another cookie.
func canonicalCookiePath(path string) bool {
	if !strings.HasPrefix(path, "/") || len(path) > 4096 {
		return false
	}
	for _, c := range path {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("/!$&'()*+,:=@-._~", c)) {
			return false
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}
