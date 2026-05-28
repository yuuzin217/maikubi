// Package model は、フロントエンドとバックエンド間のHTTP通信で使用されるデータ構造を定義します。
package model

// Response は、フロントエンドに返されるHTTPレスポンスデータを表します。
type Response struct {
	Status string `json:"status"` // Status は、HTTPステータステキスト（例: "200 OK"）です。
	Body   string `json:"body"`   // Body は、生のレスポンスボディの内容です。
	Error  string `json:"error"`  // Error は、リクエストが失敗した場合の記述的なエラーメッセージを含みます。
}

// Request は、送信されるHTTPリクエストのパラメータを表します。
type Request struct {
	Method            string `json:"method"`            // Method は、HTTPメソッド（GET, POSTなど）です。
	URL               string `json:"url"`               // URL は、送信先のエンポポイントです。
	Body              string `json:"body"`              // Body は、オプションのリクエストペイロード（通常はJSON）です。
	ProtoSchema       string `json:"protoSchema"`       // ProtoSchema は、ユーザーがアップロードした .proto のスキーマ定義です。
	ProtoRequestType  string `json:"protoRequestType"`  // ProtoRequestType は、リクエスト時のProtobufメッセージ型名です。
	ProtoResponseType string `json:"protoResponseType"` // ProtoResponseType は、レスポンス時のProtobufメッセージ型名です。
}

// Targets は、送信先のベースURL（Production, Staging, Baseline）の定義です。
// ※将来的に動的に環境数が増減する可能性がある場合は、map[string]string を使用するのが適しています。
//   今回は、この3つの環境比較に特化してコンパイラによる綴りチェック等の安全性を高めるため、構造体として定義しています。
type Targets struct {
	Production string `json:"production"` // Production は、本番環境のベースURLです。
	Staging    string `json:"staging"`    // Staging は、検証環境のベースURLです。
	Baseline   string `json:"baseline"`   // Baseline は、テスト/差分検証用のベースURLです。
}

// DiffRequest は、複数の環境（ターゲット）に対して同時に送るリクエストのパラメータを表します。
type DiffRequest struct {
	Method            string  `json:"method"`            // Method は、共通のHTTPメソッドです。
	Path              string  `json:"path"`              // Path は、各ベースURLに続く共通のパスです。
	Body              string  `json:"body"`              // Body は、共通のリクエストペイロードです。
	Targets           Targets `json:"targets"`           // Targets は、送信先のベースURLの設定です。
	ProtoSchema       string  `json:"protoSchema"`       // ProtoSchema は、ユーザーがアップロードした .proto のスキーマ定義です。
	ProtoRequestType  string  `json:"protoRequestType"`  // ProtoRequestType は、リクエスト時のProtobufメッセージ型名です。
	ProtoResponseType string  `json:"protoResponseType"` // ProtoResponseType は、レスポンス時のProtobufメッセージ型名です。
}

// Comparison は、2つのレスポンス間の差分詳細を表します。
type Comparison struct {
	SourceIndex int    `json:"sourceIndex"` // 比較元のインデックス
	TargetIndex int    `json:"targetIndex"` // 比較先のインデックス
	IsEqual     bool   `json:"isEqual"`     // ボディが完全に一致するかどうか
	Diff        string `json:"diff"`        // 差分のテキスト（差分がある場合のみ）
}

// DiffLine は、フロントエンド（React）がバーチャルスクロールで描画するための「行単位のフラットなデータ」を表します。
type DiffLine struct {
	LineNumber int    `json:"lineNumber"` // 表示上の行番号 (1-indexed)
	Status     string `json:"status"`     // "added" | "deleted" | "modified" | "matched"
	Text       string `json:"text"`       // インデントやキー・値を含む整形済みテキスト
	JSONPath   string `json:"jsonPath"`   // インラインIgnore用のJSONPath (例: "$.data.items[0].id")
}

// DiffResponse は、バックエンドで実行された比較結果を含むレスポンスです。
type DiffResponse struct {
	Responses   []Response   `json:"responses"`   // 各環境からの生のレスポンス
	Comparisons []Comparison `json:"comparisons"` // バックエンドで計算された比較結果（例: Staging vs Baseline）
	IsMatched   bool         `json:"isMatched"`   // 全体的な成功判定（例: 検証とテスト環境が一致しているか）
	DiffLines   []DiffLine   `json:"diffLines"`   // フロントエンドのバーチャルスクロールに直接流し込むデータ
}
