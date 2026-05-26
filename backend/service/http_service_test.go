package service

import (
	"context"
	"maikubi/backend/model"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newMockServer は、指定されたステータスコードとレスポンスボディを返すテスト用のモックサーバーを作成します。
// テスト終了時に自動的にサーバーをクローズするように [t.Cleanup] を使用して登録します。
func newMockServer(t *testing.T, status int, body string) *httptest.Server {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// TestHTTPService_ExecuteRequest_Get は、[HTTPService.ExecuteRequest] がクライアント層と連携して
// GETリクエストを正常に処理できることを検証します。
func TestHTTPService_ExecuteRequest_Get(t *testing.T) {
	ts := newMockServer(t, http.StatusOK, `{"id": 1}`)

	svc := NewHTTPService()
	req := model.Request{
		Method: http.MethodGet,
		URL:    ts.URL,
	}

	resp := svc.ExecuteRequest(context.Background(), req)

	if resp.Error != "" {
		t.Fatalf("Expected no error, got %s", resp.Error)
	}

	if resp.Status != "200 OK" {
		t.Errorf("Expected status 200 OK, got %s", resp.Status)
	}

	if resp.Body != `{"id": 1}` {
		t.Errorf("Expected body {\"id\": 1}, got %s", resp.Body)
	}
}

// TestHTTPService_ExecuteRequest_Post は、[HTTPService.ExecuteRequest] がボディを含む
// POSTリクエストを正常に処理できることを検証します。
func TestHTTPService_ExecuteRequest_Post(t *testing.T) {
	ts := newMockServer(t, http.StatusCreated, `{"success": true}`)

	svc := NewHTTPService()
	req := model.Request{
		Method: http.MethodPost,
		URL:    ts.URL,
		Body:   `{"test": "data"}`,
	}

	resp := svc.ExecuteRequest(context.Background(), req)

	if resp.Error != "" {
		t.Fatalf("Expected no error, got %s", resp.Error)
	}

	if resp.Status != "201 Created" {
		t.Errorf("Expected status 201 Created, got %s", resp.Status)
	}

	if resp.Body != `{"success": true}` {
		t.Errorf("Expected body {\"success\": true}, got %s", resp.Body)
	}
}

// TestHTTPService_ExecuteDiffRequest は、[HTTPService.ExecuteDiffRequest] が複数の環境にリクエストを送り、
// バックエンド側で正しく差分を計算できることを検証します。
func TestHTTPService_ExecuteDiffRequest(t *testing.T) {
	// 3つの異なるレスポンスを返すモックサーバーを用意
	ts0 := newMockServer(t, http.StatusOK, `{"v": "1.0"}`) // Production
	ts1 := newMockServer(t, http.StatusOK, `{"v": "1.1"}`) // Staging
	ts2 := newMockServer(t, http.StatusOK, `{"v": "1.1"}`) // Baseline

	svc := NewHTTPService()
	diffReq := model.DiffRequest{
		Method:  http.MethodGet,
		Path:    "/test",
		Targets: model.Targets{
			Production: ts0.URL,
			Staging:    ts1.URL,
			Baseline:   ts2.URL,
		},
	}

	resp := svc.ExecuteDiffRequest(context.Background(), diffReq)

	// 検証1: レスポンスの数が正しいか
	if len(resp.Responses) != 3 {
		t.Errorf("Expected 3 responses, got %d", len(resp.Responses))
	}

	// 検証2: Staging(1) と Baseline(2) の比較結果（一致するはず）
	foundStgVsBsl := false
	for _, cmp := range resp.Comparisons {
		if cmp.SourceIndex == 1 && cmp.TargetIndex == 2 {
			foundStgVsBsl = true
			if !cmp.IsEqual {
				t.Error("Expected Staging and Baseline to be equal")
			}
		}
	}
	if !foundStgVsBsl {
		t.Error("Comparison between Staging and Baseline not found")
	}

	// 検証3: 全体の一致フラグ（Staging == Baseline なので true のはず）
	if !resp.IsMatched {
		t.Error("Expected IsMatched to be true")
	}

	// 検証4: Production(0) と Staging(1) の比較結果（一致しないはず）
	foundProdVsStg := false
	for _, cmp := range resp.Comparisons {
		if cmp.SourceIndex == 0 && cmp.TargetIndex == 1 {
			foundProdVsStg = true
			if cmp.IsEqual {
				t.Error("Expected Production and Staging to be different")
			}
			if cmp.Diff == "" {
				t.Error("Expected diff text for different responses")
			}
		}
	}
	if !foundProdVsStg {
		t.Error("Comparison between Production and Staging not found")
	}
}

// TestHTTPService_ExecuteRequest_ErrorHandling は、[HTTPService.ExecuteRequest] が
// 様々な異常系シナリオ（無効なURL、404エラー、500エラー）を適切に捕捉し、フロントエンドに返却できることを検証します。
func TestHTTPService_ExecuteRequest_ErrorHandling(t *testing.T) {
	svc := NewHTTPService()

	// 無効なURL（ホスト名解決不可）のテスト
	t.Run("Invalid URL", func(t *testing.T) {
		req := model.Request{
			Method: http.MethodGet,
			URL:    "http://non-existent-host",
		}
		resp := svc.ExecuteRequest(context.Background(), req)
		if resp.Error == "" {
			t.Error("Expected error message for invalid URL, got empty string")
		}
	})

	// HTTP 404 Not Found のテスト
	t.Run("404 Not Found", func(t *testing.T) {
		ts := newMockServer(t, http.StatusNotFound, "Not Found")

		req := model.Request{
			Method: http.MethodGet,
			URL:    ts.URL,
		}
		resp := svc.ExecuteRequest(context.Background(), req)

		if resp.Status != "404 Not Found" {
			t.Errorf("Expected status 404 Not Found, got %s", resp.Status)
		}
		if resp.Body != "Not Found" {
			t.Errorf("Expected body Not Found, got %s", resp.Body)
		}
	})

	// HTTP 500 Internal Server Error のテスト
	t.Run("500 Internal Server Error", func(t *testing.T) {
		ts := newMockServer(t, http.StatusInternalServerError, "Server Error")

		req := model.Request{
			Method: http.MethodPost,
			URL:    ts.URL,
			Body:   `{}`,
		}
		resp := svc.ExecuteRequest(context.Background(), req)

		if resp.Status != "500 Internal Server Error" {
			t.Errorf("Expected status 500 Internal Server Error, got %s", resp.Status)
		}
		if resp.Body != "Server Error" {
			t.Errorf("Expected body Server Error, got %s", resp.Body)
		}
	})
}

// TestHTTPService_ExecuteDiffRequest_Degraded は、Staging と Baseline のレスポンスが異なる場合（デグレード発生時）、
// 全体の一致フラグ [IsMatched] が false になることを検証します。
func TestHTTPService_ExecuteDiffRequest_Degraded(t *testing.T) {
	ts0 := newMockServer(t, http.StatusOK, `{"v": "1.0"}`) // Production
	ts1 := newMockServer(t, http.StatusOK, `{"v": "1.2"}`) // Staging (変更・バグ混入)
	ts2 := newMockServer(t, http.StatusOK, `{"v": "1.1"}`) // Baseline (正常動作版)

	svc := NewHTTPService()
	diffReq := model.DiffRequest{
		Method:  http.MethodGet,
		Path:    "/test",
		Targets: model.Targets{
			Production: ts0.URL,
			Staging:    ts1.URL,
			Baseline:   ts2.URL,
		},
	}

	resp := svc.ExecuteDiffRequest(context.Background(), diffReq)

	// 検証1: Staging と Baseline が異なるため、IsMatched は false であるべき
	if resp.IsMatched {
		t.Error("Expected IsMatched to be false on degradation")
	}

	// 検証2: Staging(1) と Baseline(2) の比較結果が不一致で、差分テキストが存在すること
	foundStgVsBsl := false
	for _, cmp := range resp.Comparisons {
		if cmp.SourceIndex == 1 && cmp.TargetIndex == 2 {
			foundStgVsBsl = true
			if cmp.IsEqual {
				t.Error("Expected Staging and Baseline to be different")
			}
			if cmp.Diff == "" {
				t.Error("Expected diff text between Staging and Baseline")
			}
		}
	}
	if !foundStgVsBsl {
		t.Error("Comparison between Staging and Baseline not found")
	}
}

// TestHTTPService_ExecuteDiffRequest_ShortTargets は、ターゲットのいくつかのURLが空の場合でも、
// パニックを起こさずに安全にエラーハンドリング処理が行われることを検証します。
func TestHTTPService_ExecuteDiffRequest_ShortTargets(t *testing.T) {
	svc := NewHTTPService()

	t.Run("Empty Targets", func(t *testing.T) {
		diffReq := model.DiffRequest{
			Method:  http.MethodGet,
			Path:    "/test",
			Targets: model.Targets{},
		}
		resp := svc.ExecuteDiffRequest(context.Background(), diffReq)
		if len(resp.Responses) != 3 {
			t.Errorf("Expected 3 responses, got %d", len(resp.Responses))
		}
		for i, r := range resp.Responses {
			if r.Error == "" {
				t.Errorf("Expected error for empty target index %d, got empty", i)
			}
		}
	})

	t.Run("One Target", func(t *testing.T) {
		ts0 := newMockServer(t, http.StatusOK, `{"v": "1.0"}`)
		diffReq := model.DiffRequest{
			Method:  http.MethodGet,
			Path:    "/test",
			Targets: model.Targets{
				Production: ts0.URL,
			},
		}
		resp := svc.ExecuteDiffRequest(context.Background(), diffReq)
		if len(resp.Responses) != 3 {
			t.Errorf("Expected 3 responses, got %d", len(resp.Responses))
		}
		if resp.Responses[envProduction].Error != "" {
			t.Errorf("Expected no error for Production, got %s", resp.Responses[envProduction].Error)
		}
		if resp.Responses[envStaging].Error == "" {
			t.Error("Expected error for empty Staging")
		}
		if resp.Responses[envBaseline].Error == "" {
			t.Error("Expected error for empty Baseline")
		}
	})
}

// TestHTTPService_ExecuteDiffRequest_WithRequestError は、いくつかのリクエストが失敗した場合の
// エラーハンドリングと比較挙動を検証します。
func TestHTTPService_ExecuteDiffRequest_WithRequestError(t *testing.T) {
	ts0 := newMockServer(t, http.StatusOK, `{"v": "1.0"}`) // Production
	ts1 := newMockServer(t, http.StatusOK, `{"v": "1.1"}`) // Staging
	invalidURL := "http://non-existent-host"              // Baseline (無効なURLにしてエラーを起こす)

	svc := NewHTTPService()
	diffReq := model.DiffRequest{
		Method:  http.MethodGet,
		Path:    "/test",
		Targets: model.Targets{
			Production: ts0.URL,
			Staging:    ts1.URL,
			Baseline:   invalidURL,
		},
	}

	resp := svc.ExecuteDiffRequest(context.Background(), diffReq)

	if len(resp.Responses) != 3 {
		t.Fatalf("Expected 3 responses, got %d", len(resp.Responses))
	}

	// Baselineのレスポンスがエラーになっていることを検証
	if resp.Responses[2].Error == "" {
		t.Error("Expected error for Baseline, got empty")
	}

	// 片方がエラーで空ボディになっているため、Staging と Baseline は不一致（IsMatched = false）になるべき
	if resp.IsMatched {
		t.Error("Expected IsMatched to be false due to Baseline request error")
	}
}

// TestHTTPService_ExecuteDiffRequest_XML は、XMLレスポンスの3環境複製・比較、
// および自動ノイズカットが期待通りに動作することを検証します。
func TestHTTPService_ExecuteDiffRequest_XML(t *testing.T) {
	// 1. 各モック環境の起動
	// Production (安定版、タイムスタンプ: 12345)
	ts0 := newMockServer(t, http.StatusOK, `<response><id>100</id><name>Alice</name><role>User</role><timestamp>12345</timestamp></response>`)
	// Staging (デグレーション版、名前・ロールの変更、新規emailフィールドの追加、タイムスタンプ: 99999)
	ts1 := newMockServer(t, http.StatusOK, `<response><id>100</id><name>Alice Pro</name><role>Administrator</role><email>alice@example.com</email><timestamp>99999</timestamp></response>`)
	// Baseline (安定版、Productionと同一コードだがタイムスタンプは 67890 と動的変化)
	ts2 := newMockServer(t, http.StatusOK, `<response><id>100</id><name>Alice</name><role>User</role><timestamp>67890</timestamp></response>`)

	svc := NewHTTPService()
	diffReq := model.DiffRequest{
		Method:  http.MethodGet,
		Path:    "/xml",
		Targets: model.Targets{
			Production: ts0.URL,
			Staging:    ts1.URL,
			Baseline:   ts2.URL,
		},
	}

	resp := svc.ExecuteDiffRequest(context.Background(), diffReq)

	if len(resp.Responses) != 3 {
		t.Fatalf("Expected 3 responses, got %d", len(resp.Responses))
	}

	// 2. セマンティック比較 (Staging と Baseline は値が異なるため不一致になるべき)
	if resp.IsMatched {
		t.Error("Expected IsMatched to be false due to Staging vs Baseline regressions")
	}

	// 3. 詳細な差分行 (DiffLines) の生成とノイズカット検証
	// HTTPService でリクエストを処理後、app.goと同様に GenerateDiffLines を使って差分を分析します。
	ds := NewDiffService()
	lines, isMatched, err := ds.GenerateDiffLines(
		resp.Responses[0].Body, // Production XML
		resp.Responses[1].Body, // Staging XML
		resp.Responses[2].Body, // Baseline XML
		nil,
	)

	if err != nil {
		t.Fatalf("Expected no error from GenerateDiffLines, got %v", err)
	}

	if isMatched {
		t.Error("Expected GenerateDiffLines isMatched to be false due to diffs")
	}

	// 4. 動的ノイズカットの検証
	// Production と Baseline で timestamp (12345 vs 67890) が異なっているため、
	// 自動的にノイズパス "$.response.timestamp" として検出され、matched 状態（白文字）で出力されていることを検証します。
	timestampNoiseDetected := false
	for _, line := range lines {
		// JSONPath表現 "$.response.timestamp" になっていることを検証
		if line.JSONPath == "$.response.timestamp" {
			timestampNoiseDetected = true
			if line.Status != "matched" {
				t.Errorf("Expected timestamp noise line status to be 'matched', got '%s' (Text: %s)", line.Status, line.Text)
			}
		}
	}

	if !timestampNoiseDetected {
		t.Error("Expected '$.response.timestamp' noise path to be detected and evaluated")
	}

	// 5. デグレーションの検証
	// Stagingで追加された email フィールドが、追加差分 ('added') として検出されているか検証。
	emailAddedDetected := false
	for _, line := range lines {
		if line.JSONPath == "$.response.email" && line.Status == "added" {
			emailAddedDetected = true
		}
	}
	if !emailAddedDetected {
		t.Error("Expected '$.response.email' to be detected as 'added' difference")
	}
}
