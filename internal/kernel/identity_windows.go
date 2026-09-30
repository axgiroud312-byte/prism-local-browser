//go:build windows

package kernel

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
)

func quotedHeader(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if len(input) < 2 || input[0] != '"' {
		return "", "", errors.New("expected structured string")
	}
	value := strings.Builder{}
	for i := 1; i < len(input); i++ {
		c := input[i]
		if c == '"' {
			return value.String(), strings.TrimSpace(input[i+1:]), nil
		}
		if c == '\\' {
			i++
			if i >= len(input) || (input[i] != '\\' && input[i] != '"') {
				return "", "", errors.New("invalid escape")
			}
			c = input[i]
		}
		if c < 0x20 || c > 0x7e {
			return "", "", errors.New("invalid structured character")
		}
		value.WriteByte(c)
	}
	return "", "", errors.New("unterminated structured string")
}
func headerBrands(input string) ([]Brand, error) {
	brands := []Brand{}
	for strings.TrimSpace(input) != "" {
		name, rest, err := quotedHeader(input)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(rest, ";v=") {
			return nil, errors.New("missing version parameter")
		}
		version, rest, err := quotedHeader(rest[3:])
		if err != nil {
			return nil, err
		}
		brands = append(brands, Brand{name, version})
		if rest == "" {
			return brands, nil
		}
		if rest[0] != ',' || strings.TrimSpace(rest[1:]) == "" {
			return nil, errors.New("invalid brand list")
		}
		input = rest[1:]
	}
	return nil, errors.New("empty brand list")
}
func realBrands(brands []Brand, expected string) (map[string]string, error) {
	real := map[string]string{}
	for _, brand := range brands {
		if brand.Brand != "Google Chrome" && brand.Brand != "Chrome" && brand.Brand != "Chromium" {
			continue
		} // legal GREASE does not establish identity
		if brand.Version != expected || real[brand.Brand] != "" {
			return nil, errors.New("real brand version mismatch/duplicate")
		}
		real[brand.Brand] = brand.Version
	}
	if len(real) == 0 {
		return nil, errors.New("real brand missing")
	}
	return real, nil
}

func checkIdentity(o Observation, version string) error {
	mismatch := func() error {
		return problem("KERNEL_INTEGRITY_FAILED", "identity-mismatch", "实际程序版本与HTTP/网页UA、UA-CH身份不一致，未发布该构建。")
	}
	major := strings.Split(version, ".")[0]
	ua := regexp.MustCompile(`(?:^|\s)Chrome/([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)(?:\s|$)`).FindStringSubmatch(o.UserAgent)
	if o.BrowserVersion != version || o.HTTPUserAgent != o.UserAgent || len(ua) != 2 || (ua[1] != version && ua[1] != major+".0.0.0") || o.Platform != "Windows" || o.PlatformVersion != "15.0.0" || strings.TrimSpace(o.HTTPClientHints["sec-ch-ua-platform"]) != `"Windows"` || strings.TrimSpace(o.HTTPClientHints["sec-ch-ua-platform-version"]) != `"15.0.0"` {
		return mismatch()
	}
	for _, channel := range []struct {
		header   string
		page     []Brand
		expected string
	}{{"sec-ch-ua", o.Brands, major}, {"sec-ch-ua-full-version-list", o.FullVersionList, version}} {
		httpBrands, err := headerBrands(o.HTTPClientHints[channel.header])
		if err != nil {
			return mismatch()
		}
		httpReal, err := realBrands(httpBrands, channel.expected)
		if err != nil {
			return mismatch()
		}
		pageReal, err := realBrands(channel.page, channel.expected)
		if err != nil || !reflect.DeepEqual(httpReal, pageReal) {
			return mismatch()
		}
	}
	return nil
}
