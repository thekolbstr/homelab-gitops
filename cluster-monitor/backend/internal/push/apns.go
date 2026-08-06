package push

import (
	"encoding/json"
	"fmt"
	"sync"

	"cluster-monitor-backend/internal/alerts"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
)

// AlertNotifier sends a push notification for a health-transition event.
type AlertNotifier interface {
	Notify(evt alerts.Event) error
	RegisterDevice(deviceToken string)
	UnregisterDevice(deviceToken string)
}

type APNsSender struct {
	client    *apns2.Client
	bundleID  string
	mu        sync.RWMutex
	devices   map[string]struct{}
}

// NewAPNsSender builds a sender from a .p8 auth key. Returns an error if any
// required env var is missing — the caller should treat this as "push
// disabled" rather than fatal, so the rest of the API still works.
func NewAPNsSender(keyPath, keyID, teamID, bundleID string, production bool) (*APNsSender, error) {
	if keyPath == "" || keyID == "" || teamID == "" || bundleID == "" {
		return nil, fmt.Errorf("missing one of APNS_KEY_PATH, APNS_KEY_ID, APNS_TEAM_ID, APNS_BUNDLE_ID")
	}

	authKey, err := token.AuthKeyFromFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("reading APNs key: %w", err)
	}

	tok := &token.Token{AuthKey: authKey, KeyID: keyID, TeamID: teamID}

	client := apns2.NewTokenClient(tok)
	if production {
		client = client.Production()
	} else {
		client = client.Development()
	}

	return &APNsSender{client: client, bundleID: bundleID, devices: make(map[string]struct{})}, nil
}

func (s *APNsSender) RegisterDevice(deviceToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[deviceToken] = struct{}{}
}

func (s *APNsSender) UnregisterDevice(deviceToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.devices, deviceToken)
}

func (s *APNsSender) Notify(evt alerts.Event) error {
	// Only page for Down/Recovered — Degraded is informational (visible in
	// the app) but not worth a push.
	if evt.Status != alerts.StatusDown && evt.Status != alerts.StatusRecovered {
		return nil
	}

	title := fmt.Sprintf("%s: %s", evt.Cluster, evt.Status)
	body := fmt.Sprintf("%s/%s — %s", evt.Namespace, evt.Name, evt.Reason)

	p := payload.NewPayload().
		AlertTitle(title).
		AlertBody(body).
		Sound("default").
		Custom("eventId", evt.ID)

	s.mu.RLock()
	tokens := make([]string, 0, len(s.devices))
	for t := range s.devices {
		tokens = append(tokens, t)
	}
	s.mu.RUnlock()

	var lastErr error
	for _, deviceToken := range tokens {
		notification := &apns2.Notification{
			DeviceToken: deviceToken,
			Topic:       s.bundleID,
			Payload:     p,
		}
		res, err := s.client.Push(notification)
		if err != nil {
			lastErr = err
			continue
		}
		if !res.Sent() {
			lastErr = fmt.Errorf("apns rejected push to %s: %d %s", deviceToken, res.StatusCode, res.Reason)
		}
	}
	return lastErr
}

// PayloadJSON is a helper for debugging/logging what would be sent.
func (s *APNsSender) PayloadJSON(evt alerts.Event) string {
	b, _ := json.Marshal(evt)
	return string(b)
}
