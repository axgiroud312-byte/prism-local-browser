//go:build windows

package kernel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Immutable, private and scoped to one generated probe URL and CDP session.
// This supplies synthetic canary content, not an HTTP egress observation.
type migrationPipeRoute struct{ session, url string }

func (r *migrationPipeRoute) fulfill(ctx context.Context, p *pipeProcess, event pipeReply) error {
	var request struct {
		ID      string `json:"requestId"`
		Request struct {
			URL     string            `json:"url"`
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
		} `json:"request"`
	}
	if json.Unmarshal(event.Params, &request) != nil || request.ID == "" {
		return errors.New("invalid canary interception")
	}
	if request.Request.Method != "GET" || (request.Request.URL != r.url && request.Request.URL != r.url+"capture") {
		_, err := p.sendContext(ctx, "Fetch.failRequest", map[string]any{"requestId": request.ID, "errorReason": "BlockedByClient"}, r.session)
		return err
	}
	body := []byte("<!doctype html><title>Prism private migration canary</title>")
	contentType := "text/html; charset=utf-8"
	if request.Request.URL == r.url+"capture" {
		headers := map[string]string{}
		for name, value := range request.Request.Headers {
			switch strings.ToLower(name) {
			case "user-agent", "accept-language", "sec-ch-ua", "sec-ch-ua-platform", "sec-ch-ua-full-version-list", "sec-ch-ua-platform-version":
				headers[strings.ToLower(name)] = value
			}
		}
		body, _ = json.Marshal(headers)
		contentType = "application/json"
	}
	_, err := p.sendContext(ctx, "Fetch.fulfillRequest", map[string]any{"requestId": request.ID, "responseCode": 200, "responseHeaders": []map[string]string{{"name": "Content-Type", "value": contentType}, {"name": "Cache-Control", "value": "no-store"}, {"name": "Accept-CH", "value": "Sec-CH-UA-Full-Version-List, Sec-CH-UA-Platform-Version"}}, "body": base64.StdEncoding.EncodeToString(body)}, r.session)
	return err
}
