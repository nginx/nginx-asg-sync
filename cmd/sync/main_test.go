package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	nginx "github.com/nginx/nginx-plus-go-client/v3/client"
)

func TestGetUpstreamServerAddresses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		servers  []nginx.UpstreamServer
		expected []string
	}{
		{
			name:     "empty server list",
			servers:  []nginx.UpstreamServer{},
			expected: []string{},
		},
		{
			name: "single server",
			servers: []nginx.UpstreamServer{
				{Server: "10.0.0.1:80"},
			},
			expected: []string{"10.0.0.1:80"},
		},
		{
			name: "multiple servers",
			servers: []nginx.UpstreamServer{
				{Server: "10.0.0.1:80"},
				{Server: "10.0.0.2:80"},
				{Server: "10.0.0.3:8080"},
			},
			expected: []string{"10.0.0.1:80", "10.0.0.2:80", "10.0.0.3:8080"},
		},
		{
			name: "servers with additional fields",
			servers: []nginx.UpstreamServer{
				{Server: "192.168.1.1:443", MaxConns: intPtr(100), Weight: intPtr(5)},
				{Server: "192.168.1.2:443", MaxFails: intPtr(3)},
			},
			expected: []string{"192.168.1.1:443", "192.168.1.2:443"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getUpstreamServerAddresses(tt.servers)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d addresses, got %d", len(tt.expected), len(result))
				return
			}
			for i, addr := range result {
				if addr != tt.expected[i] {
					t.Errorf("expected address[%d] = %s, got %s", i, tt.expected[i], addr)
				}
			}
		})
	}
}

func TestGetStreamUpstreamServerAddresses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		servers  []nginx.StreamUpstreamServer
		expected []string
	}{
		{
			name:     "empty server list",
			servers:  []nginx.StreamUpstreamServer{},
			expected: []string{},
		},
		{
			name: "single stream server",
			servers: []nginx.StreamUpstreamServer{
				{Server: "10.0.0.1:3306"},
			},
			expected: []string{"10.0.0.1:3306"},
		},
		{
			name: "multiple stream servers",
			servers: []nginx.StreamUpstreamServer{
				{Server: "10.0.0.1:3306"},
				{Server: "10.0.0.2:3306"},
				{Server: "10.0.0.3:5432"},
			},
			expected: []string{"10.0.0.1:3306", "10.0.0.2:3306", "10.0.0.3:5432"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := getStreamUpstreamServerAddresses(tt.servers)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d addresses, got %d", len(tt.expected), len(result))
				return
			}
			for i, addr := range result {
				if addr != tt.expected[i] {
					t.Errorf("expected address[%d] = %s, got %s", i, tt.expected[i], addr)
				}
			}
		})
	}
}

// Helper function.
func intPtr(i int) *int {
	return &i
}

func TestCustomHeaders(t *testing.T) {
	t.Parallel()

	genHeaders := func(n int) map[string]string {
		headers := make(map[string]string)
		for i := range n {
			headerName := fmt.Sprintf("X-Custom-Header-%d", i)
			headers[headerName] = "value"
		}
		return headers
	}

	testcases := map[string]struct {
		customHeaders   map[string]string
		expectedHeaders map[string]string
		expectError     bool
	}{
		"with custom headers": {
			customHeaders: map[string]string{
				"Content-Type":    "application/json",
				"X-Custom-Header": "custom-value",
			},
			expectedHeaders: map[string]string{
				"Content-Type":    "application/json",
				"X-Custom-Header": "custom-value",
			},
		},
		"without custom headers": {},
		"maximum custom headers": {
			customHeaders:   genHeaders(maxHeaders),
			expectedHeaders: genHeaders(maxHeaders),
		},
		"too many custom headers": {
			customHeaders: genHeaders(maxHeaders + 1),
			expectError:   true,
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var gotHeaders http.Header

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			cfg := &commonConfig{
				CustomHeaders: tc.customHeaders,
			}
			client := NewHTTPClient(cfg)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			_, err = client.Do(req)
			switch {
			case tc.expectError && err == nil:
				t.Fatal("expected an error but didn't get one")
			case !tc.expectError && err != nil:
				t.Fatalf("unexpected error %v", err)
			case tc.expectError && err != nil:
				// good, nothing else to test
			case !tc.expectError && err == nil:
				for key, expectedValue := range tc.expectedHeaders {
					actualValue := gotHeaders.Get(key)
					if actualValue != expectedValue {
						t.Errorf("expected header %s = %s, got %s", key, expectedValue, actualValue)
					}
				}
			}
		})
	}
}
