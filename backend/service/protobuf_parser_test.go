package service

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
)

func TestDecodeProtobuf_DynamicLoadAndDecode(t *testing.T) {
	// 1. テスト用の .proto スキーマ定義
	schema := `
	syntax = "proto3";
	package test;

	message UserResponse {
		int32 id = 1;
		string name = 2;
		string email = 3;
		repeated string roles = 4;
		MetaInfo meta = 5;
	}

	message MetaInfo {
		string version = 1;
		string env = 2;
	}
	`

	// 2. スキーマ情報に基づいて、プログラム内で動的メッセージを組み立ててバイナリデータ（テストデータ）を作成
	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(schema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}
	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		t.Fatalf("Failed to parse schema in test setup: %v", err)
	}

	userDesc := fds[0].FindMessage("test.UserResponse")
	metaDesc := fds[0].FindMessage("test.MetaInfo")
	if userDesc == nil || metaDesc == nil {
		t.Fatalf("Failed to find message descriptors in test setup")
	}

	// MetaInfo 動的メッセージ
	metaMsg := dynamic.NewMessage(metaDesc)
	metaMsg.SetFieldByName("version", "v1.0.0")
	metaMsg.SetFieldByName("env", "production")

	// UserResponse 動的メッセージ
	userMsg := dynamic.NewMessage(userDesc)
	userMsg.SetFieldByName("id", int32(42))
	userMsg.SetFieldByName("name", "Alice")
	userMsg.SetFieldByName("email", "alice@example.com")
	userMsg.SetFieldByName("roles", []string{"Admin", "User"})
	userMsg.SetFieldByName("meta", metaMsg)

	// バイナリにエンコード
	binaryData, err := userMsg.Marshal()
	if err != nil {
		t.Fatalf("Failed to marshal test message to binary: %v", err)
	}

	// 3. 実装した DecodeProtobuf を呼び出して動的デコードを実行
	// メッセージ名はパッケージ名なしの "UserResponse" でも自動解決されるか検証します
	decoded, err := DecodeProtobuf(schema, "UserResponse", binaryData)
	if err != nil {
		t.Fatalf("DecodeProtobuf returned error: %v", err)
	}

	// 4. デコード結果（JSON互換 map[string]interface{}）の検証
	// json.Unmarshal 経由のデコードのため、数値は float64 にマッピングされます
	expected := map[string]interface{}{
		"id":    float64(42),
		"name":  "Alice",
		"email": "alice@example.com",
		"roles": []interface{}{"Admin", "User"},
		"meta": map[string]interface{}{
			"version": "v1.0.0",
			"env":     "production",
		},
	}

	if !reflect.DeepEqual(decoded, expected) {
		t.Errorf("Decoded structure mismatch.\nExpected: %+v\nGot:      %+v", expected, decoded)
	}
}

func TestDecodeProtobuf_ValidationErrors(t *testing.T) {
	schema := `syntax = "proto3"; message Dummy {}`
	
	// 空のスキーマに対するエラーハンドリング
	_, err := DecodeProtobuf("", "Dummy", []byte{})
	if err == nil {
		t.Error("Expected error for empty schema, got nil")
	}

	// 空のメッセージ名に対するエラーハンドリング
	_, err = DecodeProtobuf(schema, "", []byte{})
	if err == nil {
		t.Error("Expected error for empty message name, got nil")
	}

	// 存在しないメッセージ名に対するエラーハンドリング
	_, err = DecodeProtobuf(schema, "NonExistent", []byte{})
	if err == nil {
		t.Error("Expected error for non-existent message name, got nil")
	}
}

func TestEncodeProtobuf_DynamicEncodeAndValidation(t *testing.T) {
	schema := `
	syntax = "proto3";
	package test;

	message UserRequest {
		string name = 1;
		string role = 2;
	}
	`

	// 1. JSON入力をバイナリにエンコード
	jsonInput := `{"name":"Alice","role":"Admin"}`
	binaryData, err := EncodeProtobuf(schema, "UserRequest", jsonInput)
	if err != nil {
		t.Fatalf("EncodeProtobuf returned error: %v", err)
	}

	// 2. エンコードしたバイナリを DecodeProtobuf でデコードして検証
	decoded, err := DecodeProtobuf(schema, "UserRequest", binaryData)
	if err != nil {
		t.Fatalf("DecodeProtobuf failed to decode generated binary: %v", err)
	}

	expected := map[string]interface{}{
		"name": "Alice",
		"role": "Admin",
	}

	if !reflect.DeepEqual(decoded, expected) {
		t.Errorf("Mismatch in roundtrip encode/decode.\nExpected: %+v\nGot:      %+v", expected, decoded)
	}
}

func TestGetProtoMessageTemplate(t *testing.T) {
	schema := `
	syntax = "proto3";
	package test;

	message UserRequest {
		string name = 1;
		string role = 2;
		int32 age = 3;
		bool is_active = 4;
		repeated string tags = 5;
		Meta meta = 6;
	}

	message Meta {
		string version = 1;
	}
	`

	template, err := GetProtoMessageTemplate(schema, "UserRequest")
	if err != nil {
		t.Fatalf("GetProtoMessageTemplate returned error: %v", err)
	}

	// JSON をパースして期待通りのキーとデフォルト値になっているか検証
	var val map[string]interface{}
	if err := json.Unmarshal([]byte(template), &val); err != nil {
		t.Fatalf("Failed to unmarshal generated template: %v", err)
	}

	if val["name"] != "" {
		t.Errorf("Expected 'name' to be empty string, got: %v", val["name"])
	}
	if val["role"] != "" {
		t.Errorf("Expected 'role' to be empty string, got: %v", val["role"])
	}
	// json.Unmarshal will parse 0 as float64
	if val["age"] != float64(0) {
		t.Errorf("Expected 'age' to be 0, got: %v", val["age"])
	}
	if val["is_active"] != false {
		t.Errorf("Expected 'is_active' to be false, got: %v", val["is_active"])
	}
	if tags, ok := val["tags"].([]interface{}); !ok || len(tags) != 0 {
		t.Errorf("Expected 'tags' to be empty array, got: %v", val["tags"])
	}
	meta, ok := val["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected 'meta' to be map[string]interface{}, got: %v", val["meta"])
	}
	if meta["version"] != "" {
		t.Errorf("Expected 'meta.version' to be empty string, got: %v", meta["version"])
	}
}

func TestGetProtoMessageFields(t *testing.T) {
	schema := `
	syntax = "proto3";
	package test;

	message UserRequest {
		string name = 1;
		int32 age = 2;
		bool is_active = 3;
		repeated string tags = 4;
		Status status = 5;
		Meta meta = 6;
	}

	enum Status {
		UNKNOWN = 0;
		ACTIVE = 1;
		INACTIVE = 2;
	}

	message Meta {
		string version = 1;
	}
	`

	fields, err := GetProtoMessageFields(schema, "UserRequest")
	if err != nil {
		t.Fatalf("GetProtoMessageFields returned error: %v", err)
	}

	expected := map[string]ProtoField{
		"name":      {Name: "name", Type: "string", IsRepeated: false},
		"age":       {Name: "age", Type: "number", IsRepeated: false},
		"is_active": {Name: "is_active", Type: "bool", IsRepeated: false},
		"tags":      {Name: "tags", Type: "string", IsRepeated: true},
		"status":    {Name: "status", Type: "enum", IsRepeated: false, EnumValues: []string{"UNKNOWN", "ACTIVE", "INACTIVE"}},
		"meta":      {Name: "meta", Type: "message", IsRepeated: false},
	}

	if len(fields) != len(expected) {
		t.Fatalf("Expected %d fields, got %d", len(expected), len(fields))
	}

	for _, f := range fields {
		exp, ok := expected[f.Name]
		if !ok {
			t.Errorf("Unexpected field returned: %s", f.Name)
			continue
		}
		if f.Type != exp.Type {
			t.Errorf("Field %s: expected type %s, got %s", f.Name, exp.Type, f.Type)
		}
		if f.IsRepeated != exp.IsRepeated {
			t.Errorf("Field %s: expected IsRepeated %v, got %v", f.Name, exp.IsRepeated, f.IsRepeated)
		}
		if len(exp.EnumValues) > 0 {
			if len(f.EnumValues) != len(exp.EnumValues) {
				t.Errorf("Field %s: expected %d enum values, got %d", f.Name, len(exp.EnumValues), len(f.EnumValues))
			} else {
				for i, ev := range exp.EnumValues {
					if f.EnumValues[i] != ev {
						t.Errorf("Field %s: expected enum value %d to be %s, got %s", f.Name, i, ev, f.EnumValues[i])
					}
				}
			}
		}
	}
}
