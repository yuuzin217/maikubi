package service

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestDiffService_BasicSemanticDiff verifies that identical nested JSON objects
// are parsed, matched, and serialized correctly without diffs.
func TestDiffService_BasicSemanticDiff(t *testing.T) {
	prod := `{"user": {"name": "Alice", "age": 30}, "tags": ["admin", "staff"]}`
	staging := prod
	baseline := prod

	ds := NewDiffService()
	lines, isMatched, err := ds.GenerateDiffLines(prod, staging, baseline, nil)
	if err != nil {
		t.Fatalf("GenerateDiffLines error: %v", err)
	}

	if !isMatched {
		t.Error("Expected overall match to be true")
	}

	// すべて matched であるべき
	for _, l := range lines {
		if l.Status != "matched" {
			t.Errorf("Expected line %d status to be matched, got %s (Text: %s)", l.LineNumber, l.Status, l.Text)
		}
	}

	// 行数の目安確認 (インデントやブラケットを含むため大体12行程度)
	if len(lines) < 8 {
		t.Errorf("Expected lines to be generated, got only %d", len(lines))
	}
}

// TestDiffService_AutomaticNoiseCancellation verifies that values varying between Production
// and Baseline (e.g. dynamic timestamps) are automatically identified as noise and ignored in comparison.
func TestDiffService_AutomaticNoiseCancellation(t *testing.T) {
	prod := `{"id": 100, "timestamp": "2026-05-22T10:00:00Z", "data": "hello"}`
	baseline := `{"id": 100, "timestamp": "2026-05-22T09:59:00Z", "data": "hello"}` // タイムスタンプが異なる
	staging := `{"id": 100, "timestamp": "2026-05-22T10:05:00Z", "data": "hello"}`  // 検証環境もタイムスタンプが異なる

	ds := NewDiffService()
	lines, isMatched, err := ds.GenerateDiffLines(prod, staging, baseline, nil)
	if err != nil {
		t.Fatalf("GenerateDiffLines error: %v", err)
	}

	// タイムスタンプは「自動ノイズ」として除外されるため、全体的には一致とみなされるべき
	if !isMatched {
		t.Error("Expected IsMatched to be true due to automatic noise cancellation")
	}

	// タイムスタンプの行のステータスが "matched" になっていることを検証
	foundTimestamp := false
	for _, l := range lines {
		if strings.Contains(l.Text, "timestamp") {
			foundTimestamp = true
			if l.Status != "matched" {
				t.Errorf("Expected noise line to be matched, got %s", l.Status)
			}
		}
	}
	if !foundTimestamp {
		t.Error("timestamp field was not found in serialized lines")
	}
}

// TestDiffService_ManualIgnore verifies that manually specified JSONPaths (fully qualified)
// are successfully ignored during comparison and marked as matched.
func TestDiffService_ManualIgnore(t *testing.T) {
	prod := `{"user": {"name": "Alice", "email": "alice@example.com"}}`
	baseline := prod
	staging := `{"user": {"name": "Alice", "email": "bob@example.com"}}` // emailの値が異なる

	ds := NewDiffService()
	ignores := []string{"$.user.email"}

	// 手動Ignoreを指定
	lines, isMatched, err := ds.GenerateDiffLines(prod, staging, baseline, ignores)
	if err != nil {
		t.Fatalf("GenerateDiffLines error: %v", err)
	}

	if !isMatched {
		t.Error("Expected IsMatched to be true because difference was manually ignored")
	}

	// emailの行が matched になっていることを確認
	foundEmail := false
	for _, l := range lines {
		if strings.Contains(l.Text, "email") {
			foundEmail = true
			if l.Status != "matched" {
				t.Errorf("Expected manually ignored line to be matched, got %s", l.Status)
			}
		}
	}
	if !foundEmail {
		t.Error("email field was not found in serialized lines")
	}
}

// TestDiffService_WildcardIgnore verifies that wildcard array paths (e.g. $.items[*].price)
// successfully ignore variations in all array items at that nested structure.
func TestDiffService_WildcardIgnore(t *testing.T) {
	prod := `{"items": [{"name": "A", "price": 10}, {"name": "B", "price": 20}]}`
	baseline := prod
	staging := `{"items": [{"name": "A", "price": 12}, {"name": "B", "price": 25}]}` // priceが全て変動

	ds := NewDiffService()
	ignores := []string{"$.items[*].price"}

	lines, isMatched, err := ds.GenerateDiffLines(prod, staging, baseline, ignores)
	if err != nil {
		t.Fatalf("GenerateDiffLines error: %v", err)
	}

	if !isMatched {
		t.Error("Expected overall IsMatched to be true due to wildcard array ignore")
	}

	// 全ての price フィールドの行が matched になっているか確認
	priceCount := 0
	for _, l := range lines {
		if strings.Contains(l.Text, "price") {
			priceCount++
			if l.Status != "matched" {
				t.Errorf("Expected wildcard ignored price line to be matched, got %s (Text: %s)", l.Status, l.Text)
			}
		}
	}
	if priceCount != 2 {
		t.Errorf("Expected 2 price fields, got %d", priceCount)
	}
}

// TestDiffService_LargeJSONPerformance checks if the diffing and serialization
// of a massive JSON response (10k+ elements) takes less than 50 milliseconds in Go.
func TestDiffService_LargeJSONPerformance(t *testing.T) {
	// 巨大なJSON配列 (1000要素、それぞれ複数のネストフィールドを持つ)
	var sbB, sbS strings.Builder
	sbB.WriteString(`{"items": [`)
	sbS.WriteString(`{"items": [`)

	for i := 0; i < 1500; i++ {
		suffix := ","
		if i == 1499 {
			suffix = ""
		}
		sbB.WriteString(fmt.Sprintf(`{"id": %d, "name": "Item-%d", "values": [1, 2, 3]}%s`, i, i, suffix))
		// staging側は1箇所だけ差分を混ぜる
		if i == 500 {
			sbS.WriteString(fmt.Sprintf(`{"id": %d, "name": "Item-%d-diffed", "values": [1, 2, 3]}%s`, i, i, suffix))
		} else {
			sbS.WriteString(fmt.Sprintf(`{"id": %d, "name": "Item-%d", "values": [1, 2, 3]}%s`, i, i, suffix))
		}
	}
	sbB.WriteString(`]}`)
	sbS.WriteString(`]}`)

	baseline := sbB.String()
	staging := sbS.String()

	ds := NewDiffService()

	start := time.Now()
	lines, isMatched, err := ds.GenerateDiffLines("", staging, baseline, nil)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("GenerateDiffLines error on large payload: %v", err)
	}

	if isMatched {
		t.Error("Expected IsMatched to be false due to intentional difference at index 500")
	}

	// 1500要素 * (オブジェクトで約6行) => 約9000行以上のフラットデータが出力されるはず
	t.Logf("Generated %d lines of flat diff in %v", len(lines), duration)

	if len(lines) < 8000 {
		t.Errorf("Expected at least 8000 serialized lines, got %d", len(lines))
	}

	// パフォーマンス制約の検証 (50ms)
	limit := 50 * time.Millisecond
	if duration > limit {
		t.Errorf("Performance limit exceeded! Expected < %v, got %v", limit, duration)
	}
}
