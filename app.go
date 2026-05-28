package main

import (
	"context"
	"maikubi/backend/model"
	"maikubi/backend/service"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App は、GoバックエンドとWailsフロントエンドを橋渡しするメインアプリケーションコントローラーです。
type App struct {
	ctx         context.Context
	httpService *service.HTTPService
	diffService *service.DiffService
}

// NewApp は、[App] インスタンスを作成および初期化します。
func NewApp() *App {
	return &App{
		httpService: service.NewHTTPService(),
		diffService: service.NewDiffService(),
	}
}

// startup は、アプリケーションの開始時に呼び出されるWailsのライフサイクルホックです。
// 将来ランタイムメソッドで使用するためにコンテキストを保存します。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SendRequest は、単一のターゲットに対してHTTPリクエストを送信します。
func (a *App) SendRequest(method string, url string, body string) model.Response {
	runtime.LogInfof(a.ctx, "SendRequest called: %s %s", method, url)
	req := model.Request{
		Method: method,
		URL:    url,
		Body:   body,
	}
	resp := a.httpService.ExecuteRequest(a.ctx, req)
	if resp.Error != "" {
		runtime.LogErrorf(a.ctx, "SendRequest failed: %s", resp.Error)
	} else {
		runtime.LogInfof(a.ctx, "SendRequest succeeded with status: %s", resp.Status)
	}
	return resp
}

// SendDiffRequest は、複数の環境（ターゲット）に対して同時にリクエストを送信し、自動ノイズカットおよびフラット化されたDiffLine配列を含めて結果を返します。
func (a *App) SendDiffRequest(
	method string,
	path string,
	body string,
	targets model.Targets,
	manualIgnores []string,
	protoSchema string,
	protoReqType string,
	protoRespType string,
) model.DiffResponse {
	runtime.LogInfof(a.ctx, "SendDiffRequest started: %s %s", method, path)
	diffReq := model.DiffRequest{
		Method:            method,
		Path:              path,
		Body:              body,
		Targets:           targets,
		ProtoSchema:       protoSchema,
		ProtoRequestType:  protoReqType,
		ProtoResponseType: protoRespType,
	}
	resp := a.httpService.ExecuteDiffRequest(a.ctx, diffReq)

	// ターゲットが3つあり、かつエラーが発生していない場合、DiffLine配列を生成
	if len(resp.Responses) >= 3 && resp.Responses[0].Error == "" && resp.Responses[1].Error == "" && resp.Responses[2].Error == "" {
		runtime.LogInfo(a.ctx, "Diffing responses and generating diff lines...")
		lines, isMatched, err := a.diffService.GenerateDiffLines(
			resp.Responses[0].Body, // Production
			resp.Responses[1].Body, // Staging
			resp.Responses[2].Body, // Baseline
			manualIgnores,
		)
		if err == nil {
			resp.DiffLines = lines
			resp.IsMatched = isMatched
			runtime.LogInfof(a.ctx, "Diff computation completed. isMatched: %v, lines: %d", isMatched, len(lines))
		} else {
			runtime.LogErrorf(a.ctx, "Failed to generate diff lines: %v", err)
		}
	} else {
		runtime.LogWarning(a.ctx, "Diff skipped due to response error or insufficient targets")
	}
	return resp
}

// RecomputeDiff は、HTTPリクエストを再送信することなく、手動Ignoreリストの適用だけを施して差分行を瞬時に再計算します。
func (a *App) RecomputeDiff(prodJSON, stagingJSON, baselineJSON string, manualIgnores []string) model.DiffResponse {
	runtime.LogInfof(a.ctx, "RecomputeDiff (in-memory) called with %d ignore paths", len(manualIgnores))
	lines, isMatched, err := a.diffService.GenerateDiffLines(prodJSON, stagingJSON, baselineJSON, manualIgnores)
	if err != nil {
		runtime.LogErrorf(a.ctx, "Failed to recompute diff lines: %v", err)
		return model.DiffResponse{IsMatched: false}
	}
	runtime.LogInfof(a.ctx, "RecomputeDiff completed. isMatched: %v, lines: %d", isMatched, len(lines))
	return model.DiffResponse{
		DiffLines: lines,
		IsMatched: isMatched,
	}
}

// GetProtoMessages は、.proto スキーマ定義からメッセージタイプ名一覧を取得します。
func (a *App) GetProtoMessages(protoSchema string) ([]string, error) {
	runtime.LogInfo(a.ctx, "GetProtoMessages called")
	return service.GetProtoMessages(protoSchema)
}

// GetProtoMessageTemplate は、指定されたメッセージ名に基づき、空の JSON テンプレートを生成して返します。
func (a *App) GetProtoMessageTemplate(protoSchema string, messageName string) (string, error) {
	runtime.LogInfof(a.ctx, "GetProtoMessageTemplate called for %s", messageName)
	return service.GetProtoMessageTemplate(protoSchema, messageName)
}

// GetProtoMessageFields は、指定されたメッセージ名に基づき、フィールドのメタデータ一覧を返します。
func (a *App) GetProtoMessageFields(protoSchema string, messageName string) ([]service.ProtoField, error) {
	runtime.LogInfof(a.ctx, "GetProtoMessageFields called for %s", messageName)
	return service.GetProtoMessageFields(protoSchema, messageName)
}

// GetProtoSchemaFields は、.proto スキーマ定義内のすべてのメッセージのフィールド一覧を返します。
func (a *App) GetProtoSchemaFields(protoSchema string) (map[string][]service.ProtoField, error) {
	runtime.LogInfo(a.ctx, "GetProtoSchemaFields called")
	return service.GetProtoSchemaFields(protoSchema)
}
