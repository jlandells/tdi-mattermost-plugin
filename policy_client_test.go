package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func newPolicyTestPlugin(serverURL string) *Plugin {
	p := &Plugin{}
	p.setConfiguration(&configuration{
		TDIURL:        serverURL,
		TDINamespace:  "mattermost-policies",
		TDIAPIKey:     "test-api-key",
		PolicyTimeout: 5,
	})
	return p
}

func TestCheckPolicyAllowsAndMapsLegacyMessagePath(t *testing.T) {
	t.Parallel()

	var sawRequest bool
	p := newPolicyTestPlugin("https://tdi.example.com")
	p.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sawRequest = true
		if r.URL.Path != "/ns/mattermost-policies/policy/v1/message/check" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %q", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-api-key" {
			t.Fatalf("unexpected Authorization header %q", got)
		}
		if got := r.Header.Get(correlationIDHeader); got == "" {
			t.Fatalf("expected %s header", correlationIDHeader)
		}

		var body PolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body.Action != "message" {
			t.Fatalf("unexpected action %q", body.Action)
		}

		return jsonResponse(http.StatusOK, `{"status":"success","action":"continue","result":{}}`), nil
	})}

	allowed, reason := p.checkPolicy(PolicyRequest{Action: "message"}, "message")
	if !allowed {
		t.Fatalf("expected allow, got deny reason %q", reason)
	}
	if reason != "" {
		t.Fatalf("expected empty reason, got %q", reason)
	}
	if !sawRequest {
		t.Fatal("expected TDI request")
	}
}

func TestCheckGenericPolicyRejectReason(t *testing.T) {
	t.Parallel()

	p := newPolicyTestPlugin("https://tdi.example.com")
	p.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/ns/mattermost-policies/policy/v1/file/upload" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, `{"status":"success","action":"reject","result":{"reason":"Blocked by test policy"}}`), nil
	})}

	allowed, reason := p.checkGenericPolicy(map[string]interface{}{"action": "file_upload"}, "file/upload")
	if allowed {
		t.Fatal("expected deny")
	}
	if reason != "Blocked by test policy" {
		t.Fatalf("unexpected reason %q", reason)
	}
}

func TestCheckTDIPolicyFailsSecureOnInvalidResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantReason string
	}{
		{
			name:       "non 200",
			statusCode: http.StatusBadGateway,
			body:       `{"error":"upstream unavailable"}`,
			wantReason: "Policy service error",
		},
		{
			name:       "invalid json",
			statusCode: http.StatusOK,
			body:       `{bad-json`,
			wantReason: "Invalid policy response",
		},
		{
			name:       "unknown action",
			statusCode: http.StatusOK,
			body:       `{"status":"success","action":"maybe","result":{}}`,
			wantReason: "Invalid policy response",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newPolicyTestPlugin("https://tdi.example.com")
			p.httpClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(tt.statusCode, tt.body), nil
			})}

			allowed, reason := p.checkGenericPolicy(map[string]interface{}{"action": "test"}, "test/path")
			if allowed {
				t.Fatal("expected fail-secure deny")
			}
			if reason != tt.wantReason {
				t.Fatalf("expected reason %q, got %q", tt.wantReason, reason)
			}
		})
	}
}

func TestCheckTDIPolicyFailsSecureWithoutValidConfig(t *testing.T) {
	t.Parallel()

	p := &Plugin{}
	p.setConfiguration(&configuration{
		TDINamespace:  "mattermost-policies",
		PolicyTimeout: 5,
	})

	allowed, reason := p.checkGenericPolicy(map[string]interface{}{"action": "test"}, "test/path")
	if allowed {
		t.Fatal("expected deny")
	}
	if reason != "Policy service not configured" {
		t.Fatalf("unexpected reason %q", reason)
	}
}

func TestRedactedPolicyPayload(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"username": "alice",
		"email": "alice@example.com",
		"message": "classified message text",
		"old_message": "old classified text",
		"new_message": "new classified text",
		"channel_header": "CLEARANCE_REQUIRED=SECRET",
		"user_attributes": {"clearance": "SECRET", "department": "ops"},
		"file_hash": "abc123",
		"file_size": 42,
		"result": {"reason": "sensitive denial reason"}
	}`)

	redacted := redactedPolicyPayload(raw)

	for _, sensitive := range []string{
		"alice@example.com",
		"classified message text",
		"old classified text",
		"new classified text",
		"CLEARANCE_REQUIRED=SECRET",
		"SECRET",
		"abc123",
		"sensitive denial reason",
	} {
		if bytes.Contains([]byte(redacted), []byte(sensitive)) {
			t.Fatalf("expected %q to be redacted from %s", sensitive, redacted)
		}
	}

	for _, retained := range []string{`"username":"alice"`, `"file_size":42`} {
		if !bytes.Contains([]byte(redacted), []byte(retained)) {
			t.Fatalf("expected %q to be retained in %s", retained, redacted)
		}
	}
}

func TestRedactedPolicyPayloadHandlesNonJSON(t *testing.T) {
	t.Parallel()

	got := redactedPolicyPayload([]byte("not-json"))
	if got != "<non-json payload: 8 bytes>" {
		t.Fatalf("unexpected non-json summary %q", got)
	}
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
