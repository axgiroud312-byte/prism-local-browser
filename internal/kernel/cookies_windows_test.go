//go:build windows

package kernel

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// A synthetic pipe protocol seam; no Chromium/Job/website is created.
func syntheticCookiePipe(t *testing.T, response json.RawMessage, rejected bool) *ManagedProcess {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	p := &pipeProcess{write: writer, responses: make(chan pipeReply, 1), stopped: make(chan struct{}), commandGate: make(chan struct{}, 1)}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	go func() {
		packet, err := bufio.NewReader(reader).ReadBytes(0)
		if err != nil {
			return
		}
		var command struct {
			ID        int    `json:"id"`
			Method    string `json:"method"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(packet[:len(packet)-1], &command) != nil {
			return
		}
		if command.Method != "Storage.getCookies" || command.SessionID != "" {
			p.responses <- pipeReply{ID: command.ID, Error: json.RawMessage(`{"message":"wrong synthetic context"}`)}
			return
		}
		reply := pipeReply{ID: command.ID, Result: response}
		if rejected {
			reply.Error, reply.Result = response, nil
		}
		p.responses <- reply
	}()
	return &ManagedProcess{pipe: p}
}

func TestCookieRootReadbackKeepsSessionEmptyValueAndDoesNotReenterCommandGate(t *testing.T) {
	p := syntheticCookiePipe(t, json.RawMessage(`{"cookies":[{"name":"synthetic","value":"","domain":"example.test","path":"/","secure":true,"httpOnly":true,"session":true,"expires":-1}]}`), false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p.pipe.commandGate <- struct{}{}
	defer func() { <-p.pipe.commandGate }()
	stored, err := p.readCookiesLocked(ctx)
	if err != nil || len(stored) != 1 || stored[0].Value != "" || !stored[0].Session || stored[0].Expires == nil || *stored[0].Expires != -1 {
		t.Fatal("root Storage readback lost required Cookie semantics")
	}
}

func TestCookieMissingOrRejectedReadbackIsNotSuccessAndDoesNotEchoReply(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		p := syntheticCookiePipe(t, json.RawMessage(`{"message":"SYNTHETIC_PRIVATE_CDP_COOKIE_VALUE"}`), rejected)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		stored, err := p.readCookiesLocked(ctx)
		cancel()
		if err == nil || stored != nil || strings.Contains(err.Error(), "SYNTHETIC_PRIVATE_CDP_COOKIE_VALUE") {
			t.Fatal("missing/rejected Cookie response counted as success or leaked content")
		}
	}
}
