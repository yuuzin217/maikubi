// Package client は、標準の net/http クライアントの低レベルなラッパーを提供します。
package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultTimeout は、HTTPクライアントのデフォルトのタイムアウト秒数（30秒）です。
	DefaultTimeout = 30 * time.Second

	// HTTPヘッダー関連の定数
	headerContentType = "Content-Type"
	mimeJSON          = "application/json"
)

// HTTPClient は、基本的なリクエスト実行を処理する [http.Client] のスレッドセーフなラッパーです。
type HTTPClient struct {
	httpClient *http.Client
}

// NewHTTPClient は、指定されたタイムアウトおよび最適化された接続プール設定（Transport）で新しい [HTTPClient] を作成します。
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,              // プールする最大接続数
				MaxIdleConnsPerHost: 10,               // 同一ホストごとの最大接続数（maikubiでは3並行リクエストなどを頻繁に行うため引き上げます）
				IdleConnTimeout:     90 * time.Second, // 接続のアイドル状態タイムアウト
			},
		},
	}
}

// DoRequest は、指定されたコンテキスト、メソッド、URL、およびボディでHTTPリクエストを実行します。
// ボディが提供されている場合、"Content-Type" ヘッダーに "application/json" を設定します。
// HTTPステータス文字列、レスポンスボディ（文字列）、および発生したエラーを返します。
func (c *HTTPClient) DoRequest(ctx context.Context, method, url, body string) (string, string, error) {
	var reqBody io.Reader
	if body != "" {
		reqBody = bytes.NewBufferString(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return "", "", err
	}

	if body != "" {
		req.Header.Set(headerContentType, mimeJSON)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	return resp.Status, string(respBody), nil
}
