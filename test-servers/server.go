package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"
)

type UserRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type UserResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Email     string    `json:"email,omitempty"`     // Stagingでのみ追加される新規フィールド
	Timestamp int64     `json:"timestamp"`           // 動的ノイズ（タイムスタンプ）
	RequestID string    `json:"request_id"`          // 動的ノイズ（毎回変わるリクエストID）
	Meta      OuterMeta `json:"meta"`
}

type OuterMeta struct {
	Version   string `json:"version"`
	ServerEnv string `json:"server_env"`
}

// XML リクエスト/レスポンス用の構造体
type XMLUserRequest struct {
	XMLName xml.Name `xml:"request"`
	Name    string   `xml:"name"`
	Role    string   `xml:"role"`
}

type XMLUserResponse struct {
	XMLName   xml.Name `xml:"response"`
	ID        int      `xml:"id"`
	Name      string   `xml:"name"`
	Role      string   `xml:"role"`
	Email     string   `xml:"email,omitempty"`
	Timestamp int64    `xml:"timestamp"`
	RequestID string   `xml:"request_id"`
	Meta      XMLMeta  `xml:"meta"`
}

type XMLMeta struct {
	Version   string `xml:"version"`
	ServerEnv string `xml:"server_env"`
}

func main() {
	envName := os.Getenv("ENV_NAME")
	if envName == "" {
		envName = "development"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	rand.Seed(time.Now().UnixNano())

	// JSON 用エンドポイント
	http.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// UUID風のランダムな文字列（リクエストID）
		reqID := fmt.Sprintf("%x-%x", rand.Int31(), rand.Int31())

		name := "Alice"
		role := "User"
		var email string

		// POST リクエストの場合、リクエストボディから Name と Role を読み取る
		if r.Method == http.MethodPost {
			var req UserRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				if req.Name != "" {
					name = req.Name
				}
				if req.Role != "" {
					role = req.Role
				}
			}
		}

		// Staging環境のみ、意図的なデグレーション（機能差分）を含める
		if envName == "staging" {
			name = name + " Pro"                 // 差分：値の変更
			role = "Administrator"             // 差分：値の変更
			if r.Method == http.MethodPost {
				email = "posted.user.pro@example.com"    // 差分：新規フィールドの追加 (POST)
			} else {
				email = "alice.pro@example.com"    // 差分：新規フィールドの追加 (GET)
			}
		}

		resp := UserResponse{
			ID:        100,
			Name:      name,
			Role:      role,
			Email:     email,
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond), // 動的ノイズ
			RequestID: reqID,                                            // 動的ノイズ
			Meta: OuterMeta{
				Version:   "v1.0.0",
				ServerEnv: envName,
			},
		}

		if envName == "staging" {
			resp.Meta.Version = "v1.1.0" // 差分：ネストしたオブジェクト内の差分
		}

		json.NewEncoder(w).Encode(resp)
	})

	// XML 用エンドポイント
	http.HandleFunc("/api/v1/xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		reqID := fmt.Sprintf("%x-%x", rand.Int31(), rand.Int31())

		name := "Alice"
		role := "User"
		var email string

		// POST リクエストの場合、リクエストボディから XML を読み取る
		if r.Method == http.MethodPost {
			var req XMLUserRequest
			if err := xml.NewDecoder(r.Body).Decode(&req); err == nil {
				if req.Name != "" {
					name = req.Name
				}
				if req.Role != "" {
					role = req.Role
				}
			}
		}

		// Staging環境のみ、意図的なデグレーションを含める
		if envName == "staging" {
			name = name + " Pro"
			role = "Administrator"
			if r.Method == http.MethodPost {
				email = "posted.user.pro@example.com"
			} else {
				email = "alice.pro@example.com"
			}
		}

		resp := XMLUserResponse{
			ID:        100,
			Name:      name,
			Role:      role,
			Email:     email,
			Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
			RequestID: reqID,
			Meta: XMLMeta{
				Version:   "v1.0.0",
				ServerEnv: envName,
			},
		}

		if envName == "staging" {
			resp.Meta.Version = "v1.1.0"
		}

		w.Write([]byte(xml.Header))
		xml.NewEncoder(w).Encode(resp)
	})

	fmt.Printf("Starting mock API server [%s] on port %s...\n", envName, port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}
