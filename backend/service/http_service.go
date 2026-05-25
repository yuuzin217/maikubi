// Package service は、HTTPリクエストを処理するためのアプリケーションビジネスロジックを実装します。
package service

import (
	"context"
	"encoding/json"
	"maikubi/backend/client"
	"maikubi/backend/model"
	"sync"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const (
	// 各環境のインデックス定義
	envProduction = 0
	envStaging    = 1
	envBaseline   = 2

	// 各種比較に必要な最低レスポンス数
	requiredResponsesStagingVSBaseline = 3
	requiredResponsesProdVSStaging    = 2
)

// HTTPService は、モデル層とクライアント層を調整してHTTPタスクを実行します。
type HTTPService struct {
	client *client.HTTPClient
}

// NewHTTPService は、依存関係を初期化した新しい [HTTPService] を作成します。
func NewHTTPService() *HTTPService {
	return &HTTPService{
		client: client.NewHTTPClient(client.DefaultTimeout),
	}
}

// ExecuteRequest は、[model.Request] を処理し、[model.Response] を返します。
func (s *HTTPService) ExecuteRequest(ctx context.Context, req model.Request) model.Response {
	status, body, err := s.client.DoRequest(ctx, req.Method, req.URL, req.Body)
	if err != nil {
		return model.Response{
			Error: err.Error(),
		}
	}

	return model.Response{
		Status: status,
		Body:   body,
	}
}

// ExecuteDiffRequest は、[model.DiffRequest] を受け取り、複数のターゲットに対して並行してリクエストを実行し、
// バックエンド側でレスポンス内容の比較（Diff）を行います。
func (s *HTTPService) ExecuteDiffRequest(ctx context.Context, diffReq model.DiffRequest) model.DiffResponse {
	var wg sync.WaitGroup

	// 固定された3つの環境（Production, Staging, Baseline）のレスポンススライス
	// 定義済みの環境数（3）をサイズとして確保します。
	const targetCount = 3
	responses := make([]model.Response, targetCount)

	// ターゲット（環境）とその割り当てられたインデックスのペア定義
	targets := []struct {
		index int
		url   string
	}{
		{envProduction, diffReq.Targets.Production},
		{envStaging, diffReq.Targets.Staging},
		{envBaseline, diffReq.Targets.Baseline},
	}

	// 各ターゲットに対して並行してリクエストを実行。
	// 全ターゲットに対して無条件でGoroutineを起動するため、ループの前に一括してAddしています。
	// ※将来的にループ内で条件分岐（continue/break等）によりGoroutineを起動しないケースを追加する場合は、
	//   カウントの不一致（デッドロック）を防ぐため、ループ内で wg.Add(1) を呼ぶ形に戻す必要があります。
	wg.Add(targetCount)
	for _, target := range targets {
		go func(c context.Context, index int, baseURL string) {
			defer wg.Done()

			req := model.Request{
				Method: diffReq.Method,
				URL:    baseURL + diffReq.Path,
				Body:   diffReq.Body,
			}
			responses[index] = s.ExecuteRequest(c, req)
		}(ctx, target.index, target.url)
	}

	wg.Wait()

	comparisons := []model.Comparison{}
	isMatched := true

	// 比較ロジック：
	// インデックス 1 (Staging) と 2 (Baseline) を比較して、デプロイによるデグレードがないか確認します。
	if len(responses) >= requiredResponsesStagingVSBaseline {
		cmp := s.compare(envStaging, envBaseline, responses[envStaging].Body, responses[envBaseline].Body)
		comparisons = append(comparisons, cmp)
		isMatched = cmp.IsEqual
	}

	// オプション：インデックス 0 (Production) と 1 (Staging) を比較して、変更点を確認します。
	if len(responses) >= requiredResponsesProdVSStaging {
		cmp := s.compare(envProduction, envStaging, responses[envProduction].Body, responses[envStaging].Body)
		comparisons = append(comparisons, cmp)
	}

	return model.DiffResponse{
		Responses:   responses,
		Comparisons: comparisons,
		IsMatched:   isMatched,
	}
}

// compare は、2つのテキスト間の差分を計算します。
// JSONの場合は、キー順序の揺れや空白の違いを無視して意味的に正しく比較するため、正規化処理を挟みます。
func (s *HTTPService) compare(srcIdx, tgtIdx int, srcBody, tgtBody string) model.Comparison {
	dmp := diffmatchpatch.New()

	// JSONとして正規化した文字列を取得（JSONとして無効な場合は元の文字列が返る）
	canonicalSrc := s.canonicalJSON(srcBody)
	canonicalTgt := s.canonicalJSON(tgtBody)

	// セマンティックな一致判定には正規化後の文字列を使用
	isEqual := canonicalSrc == canonicalTgt
	diffText := ""

	if !isEqual {
		// 差分テキストの表示には、構造が揃った正規化後のJSONの差分を使用することで、
		// キーの順序揺れによるテキスト差分ノイズを完全に排除します。
		diffs := dmp.DiffMain(canonicalSrc, canonicalTgt, false)
		diffText = dmp.DiffPrettyText(diffs)
	}

	return model.Comparison{
		SourceIndex: srcIdx,
		TargetIndex: tgtIdx,
		IsEqual:     isEqual,
		Diff:        diffText,
	}
}

// canonicalJSON は、JSON文字列をキーをソートし余分な空白を排除した標準形式に正規化します。
// JSONとしてパースできない場合は、元の文字列をそのまま返します。
func (s *HTTPService) canonicalJSON(jsonStr string) string {
	var val interface{}
	if err := json.Unmarshal([]byte(jsonStr), &val); err != nil {
		return jsonStr
	}
	canonicalBytes, err := json.Marshal(val)
	if err != nil {
		return jsonStr
	}
	return string(canonicalBytes)
}
