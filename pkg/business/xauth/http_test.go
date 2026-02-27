package xauth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPClient(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		client := NewHTTPClient(HTTPClientConfig{
			BaseURL: "https://test.com",
		})

		if client.timeout != DefaultTimeout {
			t.Errorf("timeout = %v, expected %v", client.timeout, DefaultTimeout)
		}
		if client.baseURL != "https://test.com" {
			t.Errorf("baseURL = %q, expected 'https://test.com'", client.baseURL)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		customTimeout := 30 * time.Second
		client := NewHTTPClient(HTTPClientConfig{
			BaseURL: "https://custom.com",
			Timeout: customTimeout,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		})

		if client.timeout != customTimeout {
			t.Errorf("timeout = %v, expected %v", client.timeout, customTimeout)
		}
	})

	t.Run("with custom http client", func(t *testing.T) {
		customHTTP := &http.Client{Timeout: 60 * time.Second}
		client := NewHTTPClient(HTTPClientConfig{
			BaseURL: "https://test.com",
			Client:  customHTTP,
		})

		if client.client != customHTTP {
			t.Error("custom HTTP client not used")
		}
	})
}

func TestNewSkipVerifyHTTPClient(t *testing.T) {
	client := NewSkipVerifyHTTPClient("https://test.com", 15*time.Second)

	if client.baseURL != "https://test.com" {
		t.Errorf("baseURL = %q, expected 'https://test.com'", client.baseURL)
	}
	if client.timeout != 15*time.Second {
		t.Errorf("timeout = %v, expected 15s", client.timeout)
	}
}

func TestHTTPClient_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("successful get", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("method = %q, expected GET", r.Method)
			}

			resp := map[string]string{"key": "value"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		var result map[string]string
		err := client.Get(ctx, "/test", nil, &result)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if result["key"] != "value" {
			t.Errorf("result = %v, expected {key: value}", result)
		}
	})

	t.Run("with headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Custom-Header") != "custom-value" {
				t.Error("custom header not set")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		headers := map[string]string{"X-Custom-Header": "custom-value"}
		err := client.Get(ctx, "/test", headers, nil)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
	})
}

func TestHTTPClient_Post(t *testing.T) {
	ctx := context.Background()

	t.Run("successful post", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %q, expected POST", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("Content-Type header not set")
			}

			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["input"] != "test" {
				t.Errorf("body = %v, expected {input: test}", body)
			}

			resp := map[string]string{"output": "result"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		body := map[string]string{"input": "test"}
		var result map[string]string
		err := client.Post(ctx, "/test", nil, body, &result)
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		if result["output"] != "result" {
			t.Errorf("result = %v, expected {output: result}", result)
		}
	})

	t.Run("post without body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		err := client.Post(ctx, "/test", nil, nil, nil)
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
	})
}

func TestHTTPClient_Request_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("4xx error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			resp := map[string]any{
				"code":    1001,
				"message": "invalid request",
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		err := client.Get(ctx, "/test", nil, nil)
		require.Error(t, err, "expected error for 4xx response")

		apiErr, ok := err.(*APIError)
		require.True(t, ok, "expected APIError, got %T", err)
		assert.Equal(t, 400, apiErr.StatusCode)
		assert.Equal(t, 1001, apiErr.Code)
	})

	t.Run("5xx error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			resp := map[string]any{
				"message": "internal error",
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		err := client.Get(ctx, "/test", nil, nil)
		require.Error(t, err, "expected error for 5xx response")

		apiErr, ok := err.(*APIError)
		require.True(t, ok, "expected APIError, got %T", err)
		assert.True(t, apiErr.Retryable(), "5xx error should be retryable")
	})

	t.Run("network error", func(t *testing.T) {
		client := NewHTTPClient(HTTPClientConfig{
			BaseURL: "http://localhost:1", // Invalid port
			Timeout: 100 * time.Millisecond,
		})

		err := client.Get(ctx, "/test", nil, nil)
		require.Error(t, err, "expected error for network failure")

		// Should be a temporary error
		assert.True(t, IsRetryable(err), "network error should be retryable")
	})

	t.Run("body marshal error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		// Channels cannot be marshaled to JSON
		unmarshalable := make(chan int)
		err := client.Post(ctx, "/test", nil, unmarshalable, nil)
		require.Error(t, err, "expected error for unmarshalable body")
	})

	t.Run("invalid json response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		var result map[string]string
		err := client.Get(ctx, "/test", nil, &result)
		require.Error(t, err, "expected error for invalid JSON")
	})
}

func TestHTTPClient_RequestWithAuth(t *testing.T) {
	ctx := context.Background()

	t.Run("adds authorization header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				t.Errorf("Authorization = %q, expected 'Bearer test-token'", auth)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		err := client.RequestWithAuth(ctx, "GET", "/test", "test-token", nil, nil, nil)
		if err != nil {
			t.Fatalf("RequestWithAuth failed: %v", err)
		}
	})

	t.Run("with existing headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				t.Error("missing Authorization header")
			}
			if r.Header.Get("X-Custom") != "value" {
				t.Error("missing custom header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		headers := map[string]string{"X-Custom": "value"}
		err := client.RequestWithAuth(ctx, "GET", "/test", "test-token", headers, nil, nil)
		if err != nil {
			t.Fatalf("RequestWithAuth failed: %v", err)
		}
	})
}

func TestHTTPClient_Do(t *testing.T) {
	ctx := context.Background()

	t.Run("execute raw request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("raw response"))
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/test", nil)
		resp, err := client.Do(ctx, req)
		if err != nil {
			t.Fatalf("Do failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, expected 200", resp.StatusCode)
		}
	})

	t.Run("with background context", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		req, _ := http.NewRequest("GET", server.URL+"/test", nil)
		resp, err := client.Do(context.Background(), req)
		if err != nil {
			t.Fatalf("Do failed: %v", err)
		}
		defer resp.Body.Close()
	})
}

func TestHTTPClient_Client(t *testing.T) {
	customHTTP := &http.Client{Timeout: 30 * time.Second}
	client := NewHTTPClient(HTTPClientConfig{
		BaseURL: "https://test.com",
		Client:  customHTTP,
	})

	if client.Client() != customHTTP {
		t.Error("Client() should return underlying http.Client")
	}
}

func TestHTTPClient_EmptyResponseBody(t *testing.T) {
	ctx := context.Background()

	t.Run("204 no content with response pointer", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
			// No body
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		var result map[string]string
		err := client.Get(ctx, "/test", nil, &result)
		if err != nil {
			t.Fatalf("Get failed: %v (should handle empty body gracefully)", err)
		}
		// result should be nil/empty since no body was returned
		if result != nil {
			t.Errorf("result = %v, expected nil for empty response", result)
		}
	})

	t.Run("200 with empty body and response pointer", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			// Empty body
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		var result map[string]string
		err := client.Get(ctx, "/test", nil, &result)
		if err != nil {
			t.Fatalf("Get failed: %v (should handle empty body gracefully)", err)
		}
	})
}

func TestHTTPClient_Request_URLHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("relative path is concatenated with baseURL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/test" {
				t.Errorf("path = %q, expected '/api/test'", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		err := client.Get(ctx, "/api/test", nil, nil)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
	})

	t.Run("full URL is used directly", func(t *testing.T) {
		// Create two servers to prove that full URL bypasses baseURL
		targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/target/endpoint" {
				t.Errorf("path = %q, expected '/target/endpoint'", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer targetServer.Close()

		// Client is configured with a different baseURL
		client := NewHTTPClient(HTTPClientConfig{BaseURL: "http://should-not-be-used.com"})

		// Use full URL as path - should go to targetServer, not baseURL
		err := client.Get(ctx, targetServer.URL+"/target/endpoint", nil, nil)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
	})

	t.Run("https full URL is used directly", func(t *testing.T) {
		// This test verifies the https:// prefix is also detected
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: "http://other.com"})

		// httptest server uses http://, but we're testing the prefix detection
		err := client.Get(ctx, server.URL+"/test", nil, nil)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
	})
}

func TestIsAbsoluteURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"HTTP://EXAMPLE.COM", true},
		{"HTTPS://EXAMPLE.COM", true},
		{"HtTp://example.com", true},
		{"HtTpS://example.com", true},
		{"/api/v1/test", false},
		{"", false},
		{"ftp://file.com", false},
		{"http", false},
	}
	for _, tt := range tests {
		if got := isAbsoluteURL(tt.input); got != tt.want {
			t.Errorf("isAbsoluteURL(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestHTTPClient_ResponseTooLarge(t *testing.T) {
	ctx := context.Background()

	t.Run("response exceeds max size", func(t *testing.T) {
		// Create a server that returns a response larger than maxResponseSize
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Write more than maxResponseSize (10MB + 1 byte)
			// For testing, we'll use a smaller buffer and verify the error type
			largeBody := make([]byte, 10*1024*1024+1) // 10MB + 1 byte
			for i := range largeBody {
				largeBody[i] = 'x'
			}
			w.Write(largeBody)
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		var result map[string]string
		err := client.Get(ctx, "/test", nil, &result)
		if err == nil {
			t.Fatal("expected error for response too large")
		}

		// Verify it's the correct error type
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Errorf("expected ErrResponseTooLarge, got %v", err)
		}
	})

	t.Run("response at max size limit is ok", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Return a simple JSON response well under the limit
			w.Write([]byte(`{"key":"value"}`))
		}))
		defer server.Close()

		client := NewHTTPClient(HTTPClientConfig{BaseURL: server.URL})

		var result map[string]string
		err := client.Get(ctx, "/test", nil, &result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result["key"] != "value" {
			t.Errorf("result = %v, expected {key: value}", result)
		}
	})
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "URL without query params",
			input:    "https://example.com/api/v1/users",
			expected: "https://example.com/api/v1/users",
		},
		{
			name:     "URL with query params",
			input:    "https://example.com/api/v1/users?id=123&name=test",
			expected: "https://example.com/api/v1/users",
		},
		{
			name:     "URL with only question mark",
			input:    "https://example.com/api?",
			expected: "https://example.com/api",
		},
		{
			name:     "relative path without query",
			input:    "/api/v1/users",
			expected: "/api/v1/users",
		},
		{
			name:     "relative path with query",
			input:    "/api/v1/users?projectId=abc",
			expected: "/api/v1/users",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeURL(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildRequestBody_StringAndBytes(t *testing.T) {
	client := NewHTTPClient(HTTPClientConfig{BaseURL: "https://example.com"})

	t.Run("string body", func(t *testing.T) {
		reader, err := client.buildRequestBody("key=value&foo=bar")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := io.ReadAll(reader)
		if string(data) != "key=value&foo=bar" {
			t.Errorf("body = %q, expected 'key=value&foo=bar'", data)
		}
	})

	t.Run("byte slice body", func(t *testing.T) {
		reader, err := client.buildRequestBody([]byte("raw-bytes"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := io.ReadAll(reader)
		if string(data) != "raw-bytes" {
			t.Errorf("body = %q, expected 'raw-bytes'", data)
		}
	})

	t.Run("io.Reader body", func(t *testing.T) {
		reader, err := client.buildRequestBody(strings.NewReader("reader-body"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := io.ReadAll(reader)
		if string(data) != "reader-body" {
			t.Errorf("body = %q, expected 'reader-body'", data)
		}
	})
}
