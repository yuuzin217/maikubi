package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newMockServer は、リクエストハンドラを受け取り、テスト終了時に自動的にクローズされるモックサーバーを作成します。
func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

// TestHTTPClient_DoRequest_Get は、[HTTPClient.DoRequest] が正常にGETリクエストを実行し、
// レスポンスを正しく処理できることを検証します。
func TestHTTPClient_DoRequest_Get(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected method GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "success"}`))
	})

	client := NewHTTPClient(DefaultTimeout)
	status, body, err := client.DoRequest(context.Background(), http.MethodGet, ts.URL, "")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedStatus := "200 OK"
	if status != expectedStatus {
		t.Errorf("Expected status %s, got %s", expectedStatus, status)
	}

	expectedBody := `{"message": "success"}`
	if body != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, body)
	}
}

// TestHTTPClient_DoRequest_Post は、[HTTPClient.DoRequest] がJSONボディを含む
// POSTリクエストを正しく実行できることを検証します。
func TestHTTPClient_DoRequest_Post(t *testing.T) {
	expectedRequestBody := `{"name": "test"}`

	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected method POST, got %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", contentType)
		}

		body, _ := io.ReadAll(r.Body)
		if string(body) != expectedRequestBody {
			t.Errorf("Expected body %s, got %s", expectedRequestBody, string(body))
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 123}`))
	})

	client := NewHTTPClient(DefaultTimeout)
	status, body, err := client.DoRequest(context.Background(), http.MethodPost, ts.URL, expectedRequestBody)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedStatus := "201 Created"
	if status != expectedStatus {
		t.Errorf("Expected status %s, got %s", expectedStatus, status)
	}

	expectedResponseBody := `{"id": 123}`
	if body != expectedResponseBody {
		t.Errorf("Expected body %s, got %s", expectedResponseBody, body)
	}
}

// TestHTTPClient_DoRequest_Error は、[HTTPClient.DoRequest] が存在しないホストへのリクエストなど、
// ネットワーク層のエラーを正しく返却することを検証します。
func TestHTTPClient_DoRequest_Error(t *testing.T) {
	client := NewHTTPClient(DefaultTimeout)
	_, _, err := client.DoRequest(context.Background(), http.MethodGet, "http://invalid-url-that-does-not-exist.local", "")

	if err == nil {
		t.Fatal("Expected error for unreachable host, got nil")
	}
}
