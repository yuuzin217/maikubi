package service

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/types/descriptorpb"
)

// DecodeProtobuf は、.proto スキーマ定義とメッセージ名に基づいて、
// Protobuf バイナリデータを JSON 互換の汎用データ構造 (interface{}) に動的デコードします。
// これにより、バイナリ形式のデータも XML/JSON 比較エンジンにそのまま安全に流し込むことが可能になります。
func DecodeProtobuf(protoSchema string, messageName string, binaryData []byte) (interface{}, error) {
	if protoSchema == "" {
		return nil, fmt.Errorf("proto schema definition is empty")
	}
	if messageName == "" {
		return nil, fmt.Errorf("target message name is empty")
	}

	// 1. メモリ上の仮想ファイルからスキーマを動的パース (カスタム FileAccessor 関数)
	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		return nil, fmt.Errorf("failed to parse proto schema: %w", err)
	}

	if len(fds) == 0 {
		return nil, fmt.Errorf("no file descriptor returned from parser")
	}

	// 2. メッセージ名に合致する記述子 (Descriptor) を検索
	msgDesc := fds[0].FindMessage(messageName)
	if msgDesc == nil {
		// パッケージ名が省略されている（メッセージ名単体）場合を考慮して GetMessageTypes から線形探索
		for _, md := range fds[0].GetMessageTypes() {
			if md.GetName() == messageName {
				msgDesc = md
				break
			}
		}
	}

	if msgDesc == nil {
		return nil, fmt.Errorf("message descriptor not found for name: %s", messageName)
	}

	// 3. 動的メッセージインスタンスを作成しバイナリをアンマーシャル
	dynMsg := dynamic.NewMessage(msgDesc)
	if err := dynMsg.Unmarshal(binaryData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal protobuf binary: %w", err)
	}

	// 4. JSON にシリアライズしたのち、汎用マップに変換して返す
	jsonBytes, err := dynMsg.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal dynamic message to JSON: %w", err)
	}

	var val interface{}
	if err := json.Unmarshal(jsonBytes, &val); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to generic structure: %w", err)
	}

	return val, nil
}

// EncodeProtobuf は、.proto スキーマ定義とメッセージ名に基づいて、
// JSON文字列を Protobuf バイナリデータに動的エンコードします。
func EncodeProtobuf(protoSchema string, messageName string, jsonInput string) ([]byte, error) {
	if protoSchema == "" {
		return nil, fmt.Errorf("proto schema definition is empty")
	}
	if messageName == "" {
		return nil, fmt.Errorf("target message name is empty")
	}
	if jsonInput == "" {
		return nil, fmt.Errorf("json input is empty")
	}

	// 1. メモリ上の仮想ファイルからスキーマを動的パース (カスタム FileAccessor 関数)
	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		return nil, fmt.Errorf("failed to parse proto schema: %w", err)
	}

	if len(fds) == 0 {
		return nil, fmt.Errorf("no file descriptor returned from parser")
	}

	// 2. メッセージ名に合致する記述子 (Descriptor) を検索
	msgDesc := fds[0].FindMessage(messageName)
	if msgDesc == nil {
		// パッケージ名が省略されている（メッセージ名単体）場合を考慮して GetMessageTypes から線形探索
		for _, md := range fds[0].GetMessageTypes() {
			if md.GetName() == messageName {
				msgDesc = md
				break
			}
		}
	}

	if msgDesc == nil {
		return nil, fmt.Errorf("message descriptor not found for name: %s", messageName)
	}

	// 3. 動的メッセージインスタンスを作成し JSON をアンマーシャル
	dynMsg := dynamic.NewMessage(msgDesc)
	if err := dynMsg.UnmarshalJSON([]byte(jsonInput)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to protobuf: %w", err)
	}

	// 4. バイナリデータをエンコード
	binaryData, err := dynMsg.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal dynamic message to binary: %w", err)
	}

	return binaryData, nil
}

// GetProtoMessages は、.proto スキーマ定義を動的解析し、定義されているメッセージタイプ名の一覧を返します。
func GetProtoMessages(protoSchema string) ([]string, error) {
	if protoSchema == "" {
		return nil, fmt.Errorf("schema content is empty")
	}

	// 1. メモリ上の仮想ファイルからスキーマを動的パース
	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		return nil, fmt.Errorf("failed to parse proto schema: %w", err)
	}

	if len(fds) == 0 {
		return nil, nil
	}

	// 2. 定義されているメッセージ名の一覧を抽出
	var messageNames []string
	for _, md := range fds[0].GetMessageTypes() {
		messageNames = append(messageNames, md.GetName())
	}

	return messageNames, nil
}

// GetProtoMessageTemplate は、指定されたメッセージ名に基づき、空の JSON テンプレートを生成して返します。
func GetProtoMessageTemplate(protoSchema string, messageName string) (string, error) {
	if protoSchema == "" {
		return "", fmt.Errorf("proto schema definition is empty")
	}
	if messageName == "" {
		return "", fmt.Errorf("target message name is empty")
	}

	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		return "", fmt.Errorf("failed to parse proto schema: %w", err)
	}

	if len(fds) == 0 {
		return "", fmt.Errorf("no file descriptor returned from parser")
	}

	msgDesc := fds[0].FindMessage(messageName)
	if msgDesc == nil {
		for _, md := range fds[0].GetMessageTypes() {
			if md.GetName() == messageName {
				msgDesc = md
				break
			}
		}
	}

	if msgDesc == nil {
		return "", fmt.Errorf("message descriptor not found for name: %s", messageName)
	}

	visited := make(map[string]bool)
	templateMap := buildTemplateMap(msgDesc, visited)

	jsonBytes, err := json.MarshalIndent(templateMap, "", "    ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal template to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

func buildTemplateMap(md *desc.MessageDescriptor, visited map[string]bool) map[string]interface{} {
	if visited[md.GetFullyQualifiedName()] {
		return nil
	}
	visited[md.GetFullyQualifiedName()] = true
	defer func() { visited[md.GetFullyQualifiedName()] = false }()

	res := make(map[string]interface{})
	for _, fd := range md.GetFields() {
		name := fd.GetName()
		if fd.IsRepeated() {
			if fd.IsMap() {
				res[name] = map[string]interface{}{}
			} else {
				res[name] = []interface{}{}
			}
			continue
		}

		if fd.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
			subMd := fd.GetMessageType()
			if subMd != nil {
				res[name] = buildTemplateMap(subMd, visited)
			} else {
				res[name] = nil
			}
		} else {
			switch fd.GetType() {
			case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
				res[name] = false
			case descriptorpb.FieldDescriptorProto_TYPE_STRING:
				res[name] = ""
			case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
				res[name] = ""
			case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
				enumDesc := fd.GetEnumType()
				if enumDesc != nil && len(enumDesc.GetValues()) > 0 {
					res[name] = enumDesc.GetValues()[0].GetName()
				} else {
					res[name] = ""
				}
			default:
				// int32, int64, float, double etc.
				res[name] = 0
			}
		}
	}
	return res
}

// ProtoField は、Protobuf スキーマ内の個別のフィールド定義メタデータを保持します。
type ProtoField struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`                 // "string", "number", "bool", "message", "enum"
	TypeName   string   `json:"typeName,omitempty"`   // 入れ子メッセージの型名
	IsRepeated bool     `json:"isRepeated"`
	EnumValues []string `json:"enumValues,omitempty"`
}

// GetProtoMessageFields は、指定されたメッセージ名に基づき、フィールドのメタデータ一覧を返します。
func GetProtoMessageFields(protoSchema string, messageName string) ([]ProtoField, error) {
	if protoSchema == "" {
		return nil, fmt.Errorf("proto schema definition is empty")
	}
	if messageName == "" {
		return nil, fmt.Errorf("target message name is empty")
	}

	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		return nil, fmt.Errorf("failed to parse proto schema: %w", err)
	}

	if len(fds) == 0 {
		return nil, fmt.Errorf("no file descriptor returned from parser")
	}

	msgDesc := fds[0].FindMessage(messageName)
	if msgDesc == nil {
		for _, md := range fds[0].GetMessageTypes() {
			if md.GetName() == messageName {
				msgDesc = md
				break
			}
		}
	}

	if msgDesc == nil {
		return nil, fmt.Errorf("message descriptor not found for name: %s", messageName)
	}

	var fields []ProtoField
	for _, fd := range msgDesc.GetFields() {
		pf := ProtoField{
			Name:       fd.GetName(),
			IsRepeated: fd.IsRepeated(),
		}

		switch fd.GetType() {
		case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
			pf.Type = "bool"
		case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES:
			pf.Type = "string"
		case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
			pf.Type = "enum"
			enumDesc := fd.GetEnumType()
			if enumDesc != nil {
				for _, ev := range enumDesc.GetValues() {
					pf.EnumValues = append(pf.EnumValues, ev.GetName())
				}
			}
		case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
			pf.Type = "message"
			if fd.GetMessageType() != nil {
				pf.TypeName = fd.GetMessageType().GetName()
			}
		default:
			pf.Type = "number"
		}
		fields = append(fields, pf)
	}

	return fields, nil
}

// GetProtoSchemaFields は、.proto スキーマ定義内のすべてのメッセージについて、
// フィールドのメタデータ一覧をメッセージ名をキーとするマップ形式で返します。
func GetProtoSchemaFields(protoSchema string) (map[string][]ProtoField, error) {
	if protoSchema == "" {
		return nil, fmt.Errorf("proto schema definition is empty")
	}

	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(protoSchema)), nil
			}
			return nil, fmt.Errorf("file not found: %s", filename)
		},
	}

	fds, err := parser.ParseFiles("schema.proto")
	if err != nil {
		return nil, fmt.Errorf("failed to parse proto schema: %w", err)
	}

	if len(fds) == 0 {
		return nil, fmt.Errorf("no file descriptor returned from parser")
	}

	res := make(map[string][]ProtoField)
	for _, md := range fds[0].GetMessageTypes() {
		msgName := md.GetName()
		var fields []ProtoField
		for _, fd := range md.GetFields() {
			pf := ProtoField{
				Name:       fd.GetName(),
				IsRepeated: fd.IsRepeated(),
			}

			switch fd.GetType() {
			case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
				pf.Type = "bool"
			case descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES:
				pf.Type = "string"
			case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
				pf.Type = "enum"
				enumDesc := fd.GetEnumType()
				if enumDesc != nil {
					for _, ev := range enumDesc.GetValues() {
						pf.EnumValues = append(pf.EnumValues, ev.GetName())
					}
				}
			case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
				pf.Type = "message"
				if fd.GetMessageType() != nil {
					pf.TypeName = fd.GetMessageType().GetName()
				}
			default:
				pf.Type = "number"
			}
			fields = append(fields, pf)
		}
		res[msgName] = fields
	}

	return res, nil
}
