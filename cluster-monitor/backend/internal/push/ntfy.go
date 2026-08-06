package push

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cluster-monitor-backend/internal/alerts"
)

// NtfySender sends alerts to an ntfy.sh topic (or a self-hosted ntfy
// instance). No Apple Developer account, no device tokens, no APNs — you
// just subscribe to the topic in the free ntfy app on your phone.
//
// Docs: https://docs.ntfy.sh/publish/
type NtfySender struct {
	baseURL string // e.g. "https://ntfy.sh" or "https://ntfy.yourdomain.com"
	topic   string // e.g. a long random string, treat it like a shared secret
	client  *http.Client
	token   string // optional: access token if your ntfy server requires auth
}

func NewNtfySender(baseURL, topic, token string) (*NtfySender, error) {
	if baseURL == "" || topic == "" {
		return nil, fmt.Errorf("missing NTFY_URL or NTFY_TOPIC")
	}
	return &NtfySender{
		baseURL: strings.TrimRight(baseURL, "/"),
		topic:   topic,
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (n *NtfySender) Notify(evt alerts.Event) error {
	// Only page for Down/Recovered — Degraded is visible in the app but not
	// worth an interrupt.
	if evt.Status != alerts.StatusDown && evt.Status != alerts.StatusRecovered {
		return nil
	}

	title := fmt.Sprintf("%s: %s", evt.Cluster, strings.ToUpper(string(evt.Status)))
	body := fmt.Sprintf("%s/%s — %s", evt.Namespace, evt.Name, evt.Reason)
	if evt.Message != "" {
		body += "\n" + evt.Message
	}

	req, err := http.NewRequest("POST", n.baseURL+"/"+n.topic, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	if evt.Status == alerts.StatusDown {
		req.Header.Set("Priority", "urgent")
		req.Header.Set("Tags", "rotating_light")
	} else {
		req.Header.Set("Priority", "default")
		req.Header.Set("Tags", "white_check_mark")
	}
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}

	res, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned status %d", res.StatusCode)
	}
	return nil
}

// RegisterDevice/UnregisterDevice are no-ops for ntfy — there's no device
// token, you subscribe to the topic directly in the ntfy app. Kept so
// NtfySender can satisfy the same shape the API layer expects.
func (n *NtfySender) RegisterDevice(string)   {}
func (n *NtfySender) UnregisterDevice(string) {}
