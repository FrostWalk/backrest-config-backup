package healthcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Notifier struct {
	client  *http.Client
	baseURL string
}

func NewNotifier(client *http.Client, baseURL string) *Notifier {
	return &Notifier{
		client:  client,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
}

func (n *Notifier) PingSuccess(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, n.baseURL, nil)
	if err != nil {
		return fmt.Errorf("creating success ping request: %w", err)
	}
	return n.do(request)
}

func (n *Notifier) PingFailure(ctx context.Context, reason string) error {
	payload := strings.NewReader(strings.TrimSpace(reason))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/fail", payload)
	if err != nil {
		return fmt.Errorf("creating failure ping request: %w", err)
	}
	request.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return n.do(request)
}

func (n *Notifier) do(request *http.Request) error {
	response, err := n.client.Do(request)
	if err != nil {
		return fmt.Errorf("sending healthchecks ping: %w", err)
	}
	defer response.Body.Close()

	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("healthchecks responded with status %d", response.StatusCode)
	}
	return nil
}
