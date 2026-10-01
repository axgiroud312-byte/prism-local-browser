package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

const MaxImportBytes = 2 << 20 // an input-memory bound, never a saved-node quota

func safeText(text string, max int) bool {
	return utf8.ValidString(text) && len(text) <= max && !strings.ContainsFunc(text, unicode.IsControl)
}

func Normalize(config Configuration) (Configuration, error) {
	config.Name, config.Type, config.Country = strings.TrimSpace(config.Name), strings.ToLower(strings.TrimSpace(config.Type)), strings.TrimSpace(config.Country)
	if !safeText(config.Name, 256) || config.Name == "" || !safeText(config.Country, 128) {
		return config, errors.New("名称或地区标签无效。")
	}
	if config.Type != "http" && config.Type != "https" && config.Type != "socks5" {
		return config, errors.New("仅支持HTTP、HTTPS和SOCKS5配置。")
	}
	if config.Port < 1 || config.Port > 65535 {
		return config, errors.New("端口须为1–65535。")
	}
	host := strings.TrimSpace(config.Host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if host == "" || strings.ContainsAny(host, "/\\@?#%") || strings.ContainsFunc(host, unicode.IsSpace) || !safeText(host, 1024) {
		return config, errors.New("地址仅填写主机名或IP，不含协议、路径或凭据。")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		config.Host = ip.String()
		return config, nil
	}
	if strings.Contains(host, ":") {
		return config, errors.New("IPv6地址无效；文本导入请使用带方括号的URI。")
	}
	host, err := idna.Lookup.ToASCII(strings.TrimSuffix(host, "."))
	if err != nil || len(host) > 253 || host == "" {
		return config, errors.New("主机名无效。")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return config, errors.New("主机名无效。")
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return config, errors.New("主机名无效。")
			}
		}
	}
	config.Host = strings.ToLower(host)
	return config, nil
}

func ValidateCredentials(credentials Credentials) error {
	if !safeText(credentials.Username, 4096) || !safeText(credentials.Password, 4096) || strings.Contains(credentials.Username, ":") || credentials.Username == "" && credentials.Password == "" {
		return errors.New("认证字段无效；用户名不能含冒号或控制字符，空凭据请明确选择清除认证。")
	}
	return nil
}

func Endpoint(config Configuration) string {
	return config.Type + "://" + net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
}

func ParseLine(raw string) (Configuration, *Credentials, error) {
	text := strings.TrimSpace(raw)
	if !strings.Contains(text, "://") {
		if strings.HasPrefix(text, "[") {
			text = "http://" + text
		} else {
			parts := strings.Split(text, ":")
			switch len(parts) {
			case 2:
				text = "http://" + text
			case 4:
				text = "http://" + url.UserPassword(parts[2], parts[3]).String() + "@" + parts[0] + ":" + parts[1]
			default:
				return Configuration{}, nil, errors.New("请使用代理URI或host:port:用户名:密码；IPv6和含分隔符的凭据使用URI编码。")
			}
		}
	}
	u, err := url.Parse(text)
	if err != nil || u.Opaque != "" || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return Configuration{}, nil, errors.New("代理URI无效；不得包含请求路径、查询或片段，特殊凭据请按URI编码。")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return Configuration{}, nil, errors.New("需要显式有效端口。")
	}
	config := Configuration{Type: strings.ToLower(u.Scheme), Host: u.Hostname(), Port: port}
	config.Name = fmt.Sprintf("%s://%s", config.Type, net.JoinHostPort(config.Host, strconv.Itoa(port)))
	config, err = Normalize(config)
	if err != nil {
		return config, nil, err
	}
	var credentials *Credentials
	if u.User != nil {
		password, _ := u.User.Password()
		value := Credentials{Username: u.User.Username(), Password: password}
		if err := ValidateCredentials(value); err != nil {
			return config, nil, err
		}
		credentials = &value
	}
	return config, credentials, nil
}

// No raw input or parser error containing raw input leaves the host service.
func ParseImport(text string) ([]Candidate, int, error) {
	if !utf8.ValidString(text) || len(text) > MaxImportBytes {
		return nil, 0, errors.New("导入文本需为UTF-8且不超过2MiB；请分文件导入。")
	}
	rows, ignored := []Candidate{}, 0
	for index, raw := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			ignored++
			continue
		}
		config, credentials, err := ParseLine(raw)
		row := Candidate{Line: index + 1, Configuration: config, Credentials: credentials}
		if err != nil {
			row.Configuration, row.Credentials, row.Error = Configuration{}, nil, err.Error()
		}
		rows = append(rows, row)
	}
	return rows, ignored, nil
}
