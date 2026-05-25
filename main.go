// Package main は、maikubi Wailsアプリケーションのエントリポイントです。
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// assets は、バイナリに埋め込まれるビルド済みのフロントエンドファイルを含みます。
//
//go:embed all:frontend/dist
var assets embed.FS

// main は、Wailsアプリケーションを構成および起動します。
func main() {
	// アプリケーション構造体のインスタンスを作成
	app := NewApp()

	// オプションを指定してアプリケーションを作成
	err := wails.Run(&options.App{
		Title:  "maikubi",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
