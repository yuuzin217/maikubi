package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
)

const protoSchema = `
syntax = "proto3";

package maikubi;

message UserRequest {
    string name = 1;
    string role = 2;
}

message UserResponse {
    int32 id = 1;
    string name = 2;
    string role = 3;
    string email = 4;
    int64 timestamp = 5;
    string request_id = 6;
    Meta meta = 7;
}

message Meta {
    string version = 1;
    string server_env = 2;
}
`

var (
	userRequestDesc  *desc.MessageDescriptor
	userResponseDesc *desc.MessageDescriptor
	metaDesc         *desc.MessageDescriptor
)

func initProto() {
	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "user.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}
	fds, err := parser.ParseFiles("user.proto")
	if err != nil {
		panic(fmt.Sprintf("failed to parse proto schema: %v", err))
	}
	fd := fds[0]
	userRequestDesc = fd.FindMessage("maikubi.UserRequest")
	userResponseDesc = fd.FindMessage("maikubi.UserResponse")
	metaDesc = fd.FindMessage("maikubi.Meta")
	if userRequestDesc == nil || userResponseDesc == nil || metaDesc == nil {
		panic("failed to find message descriptors in proto schema")
	}
}

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
	initProto()
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

	// Protobuf 用エンドポイント
	http.HandleFunc("/api/v1/proto", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")

		reqID := fmt.Sprintf("%x-%x", rand.Int31(), rand.Int31())

		name := "Alice"
		role := "User"
		var email string

		// POST リクエストの場合、リクエストボディから Protobuf をデコード
		if r.Method == http.MethodPost {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil && len(bodyBytes) > 0 {
				reqMsg := dynamic.NewMessage(userRequestDesc)
				if err := reqMsg.Unmarshal(bodyBytes); err == nil {
					reqName, _ := reqMsg.TryGetFieldByName("name")
					reqRole, _ := reqMsg.TryGetFieldByName("role")
					if reqName != nil && reqName.(string) != "" {
						name = reqName.(string)
					}
					if reqRole != nil && reqRole.(string) != "" {
						role = reqRole.(string)
					}
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

		// レスポンスの構築
		respMsg := dynamic.NewMessage(userResponseDesc)
		respMsg.SetFieldByName("id", int32(100))
		respMsg.SetFieldByName("name", name)
		respMsg.SetFieldByName("role", role)
		if email != "" {
			respMsg.SetFieldByName("email", email)
		}
		respMsg.SetFieldByName("timestamp", time.Now().UnixNano()/int64(time.Millisecond))
		respMsg.SetFieldByName("request_id", reqID)

		// ネストした Meta メッセージの構築
		metaMsg := dynamic.NewMessage(metaDesc)
		version := "v1.0.0"
		if envName == "staging" {
			version = "v1.1.0"
		}
		metaMsg.SetFieldByName("version", version)
		metaMsg.SetFieldByName("server_env", envName)

		respMsg.SetFieldByName("meta", metaMsg)

		// シリアライズ
		respBytes, err := respMsg.Marshal()
		if err != nil {
			http.Error(w, "Failed to marshal protobuf", http.StatusInternalServerError)
			return
		}

		w.Write(respBytes)
	})

	fmt.Printf("Starting mock API server [%s] on port %s...\n", envName, port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
	}
}
