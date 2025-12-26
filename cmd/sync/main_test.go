package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nginx "github.com/nginx/nginx-plus-go-client/v2/client"
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

func TestNewHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		config          *commonConfig
		expectedHeaders map[string]string
		name            string
	}{
		{
			name: "with custom headers",
			config: &commonConfig{
				CustomHeaders: map[string]string{
					"Content-Type":    "application/json",
					"X-Custom-Header": "custom-value",
					"User-Agent":      "nginx-asg-sync/v1.0",
				},
			},
			expectedHeaders: map[string]string{
				"Content-Type":    "application/json",
				"X-Custom-Header": "custom-value",
				"User-Agent":      "nginx-asg-sync/v1.0",
			},
		},
		{
			name:            "without custom headers",
			config:          &commonConfig{},
			expectedHeaders: map[string]string{},
		},
		{
			name: "with empty custom headers map",
			config: &commonConfig{
				CustomHeaders: map[string]string{},
			},
			expectedHeaders: map[string]string{},
		},
		{
			name: "with single custom header",
			config: &commonConfig{
				CustomHeaders: map[string]string{
					"Authorization": "Bearer token123",
				},
			},
			expectedHeaders: map[string]string{
				"Authorization": "Bearer token123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			headers := NewHeaders(tt.config)

			// Check that we have the expected number of headers
			if len(headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d", len(tt.expectedHeaders), len(headers))
			}

			// Check each expected header
			for expectedKey, expectedValue := range tt.expectedHeaders {
				actualValue := headers.Get(expectedKey)
				if actualValue != expectedValue {
					t.Errorf("expected header %s = %s, got %s", expectedKey, expectedValue, actualValue)
				}
			}

			// Check that no unexpected headers are present
			for actualKey := range headers {
				if _, expected := tt.expectedHeaders[actualKey]; !expected {
					t.Errorf("unexpected header %s found", actualKey)
				}
			}
		})
	}
}

func TestHeaderTransport_RoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		headers            http.Header
		name               string
		expectedStatusCode int
		expectError        bool
	}{
		{
			name: "successful request with headers",
			headers: http.Header{
				"Content-Type":  []string{"application/json"},
				"Authorization": []string{"ApiKey test123"},
			},
			expectedStatusCode: http.StatusOK,
			expectError:        false,
		},
		{
			name: "request without custom headers",
			headers: http.Header{
				"Content-Type": []string{"application/json"},
			},
			expectedStatusCode: http.StatusOK,
			expectError:        false,
		},
		{
			name:               "request with empty headers",
			headers:            http.Header{},
			expectedStatusCode: http.StatusOK,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify custom headers were added
				for key := range tt.headers {
					if r.Header.Get(key) == "" {
						t.Errorf("expected header %s to be present", key)
					}
				}
				w.WriteHeader(tt.expectedStatusCode)
				if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer server.Close()

			transport := &headerTransport{
				headers:   tt.headers,
				transport: http.DefaultTransport,
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := transport.RoundTrip(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatusCode {
				t.Errorf("expected status %d, got %d", tt.expectedStatusCode, resp.StatusCode)
			}
		})
	}
}

func TestHeaderTransport_RoundTrip_HeaderLimits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		errorMessage string
		numHeaders   int
		expectError  bool
	}{
		{
			name:        "well below header limit",
			numHeaders:  10,
			expectError: false,
		},
		{
			name:        "at half header limit",
			numHeaders:  maxHeaders / 2,
			expectError: false,
		},
		{
			name:        "exactly at header limit",
			numHeaders:  maxHeaders,
			expectError: false,
		},
		{
			name:         "one over header limit",
			numHeaders:   maxHeaders + 1,
			expectError:  true,
			errorMessage: "number of headers in request exceeds the maximum allowed",
		},
		{
			name:        "zero headers",
			numHeaders:  0,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer server.Close()

			headers := http.Header{}
			for i := range tt.numHeaders {
				headerName := fmt.Sprintf("X-Custom-Header-%d", i)
				headers.Add(headerName, "value")
			}

			transport := &headerTransport{
				headers:   headers,
				transport: http.DefaultTransport,
			}

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			resp, err := transport.RoundTrip(req)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for %d headers, got nil", tt.numHeaders)
				} else if tt.errorMessage != "" && !strings.Contains(err.Error(), tt.errorMessage) {
					t.Errorf("expected error message to contain %q, got %q", tt.errorMessage, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for %d headers: %v", tt.numHeaders, err)
				}
				if resp != nil {
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Errorf("expected status 200, got %d", resp.StatusCode)
					}
				}
			}
		})
	}
}

func TestNewHTTPClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		config             *commonConfig
		name               string
		hasHeaderTransport bool
	}{
		{
			name: "with custom headers",
			config: &commonConfig{
				CustomHeaders: map[string]string{
					"Content-Type": "application/json",
					"User-Agent":   "nginx-asg-sync/v1.0",
				},
			},
			hasHeaderTransport: true,
		},
		{
			name:               "without custom headers",
			config:             &commonConfig{},
			hasHeaderTransport: false,
		},
		{
			name: "with empty custom headers map",
			config: &commonConfig{
				CustomHeaders: map[string]string{},
			},
			hasHeaderTransport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := NewHTTPClient(tt.config)

			if client == nil {
				t.Fatal("expected non-nil client")
			}

			if client.Timeout != connTimeoutInSecs*1000000000 { // nanoseconds
				t.Errorf("expected timeout %v, got %v", connTimeoutInSecs*1000000000, client.Timeout)
			}

			if client.Transport == nil {
				t.Error("expected non-nil transport")
			}

			if tt.hasHeaderTransport {
				if _, ok := client.Transport.(*headerTransport); !ok {
					t.Error("expected transport to be *headerTransport when custom headers are present")
				}
			} else {
				if _, ok := client.Transport.(*headerTransport); ok {
					t.Error("expected transport to be *http.Transport when no custom headers are present")
				}
			}
		})
	}
}

// Helper function.
func intPtr(i int) *int {
	return &i
}
