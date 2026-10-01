package proxy

import (
	"context"
	"io"
	"sync"
)

// Request cancellation is not evidence that the bound upstream has failed.
// In particular, cancellation callbacks close sockets during TLS/SOCKS reads.
func requestContextFailure(ctx context.Context, failure *CheckError) *CheckError {
	if failure == nil || ctx.Err() == nil {
		return failure
	}
	if ctx.Err() == context.DeadlineExceeded {
		return &CheckError{Code: "PROXY_REQUEST_TIMEOUT", Message: "本次请求超过时限，仅收敛本次连接，未判定整个代理失效。", Retryable: true}
	}
	return &CheckError{Code: "OPERATION_CANCELLED", Message: "本次请求已取消，仅收敛本次连接，未切直连或结束其他请求。", Retryable: true}
}

// Request.Write[Proxy] can fail while reading the browser's upload, not just
// while writing the upstream socket. Preserve the source of that failure.
type proxyRequestBody struct {
	io.ReadCloser
	mu             sync.Mutex
	expected, read int64
	failed         bool
}

func (b *proxyRequestBody) Read(value []byte) (int, error) {
	n, err := b.ReadCloser.Read(value)
	b.mu.Lock()
	b.read += int64(n)
	if err != nil && (err != io.EOF || b.expected > 0 && b.read < b.expected) {
		b.failed = true
	}
	b.mu.Unlock()
	return n, err
}

func (b *proxyRequestBody) sourceFailed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failed
}
