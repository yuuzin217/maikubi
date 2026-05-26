package service

import (
	"encoding/xml"
	"io"
	"strings"
)

const (
	xmlAttrPrefix = "@"
	xmlTextKey    = "#text"
)

// ParseXMLToMap は、XML文字列を map[string]interface{} や []interface{}、string などのJSON互換構造に変換します。
// 二重実装を避け、既存の JSON 比較エンジン (DiffService) にそのまま渡して差分計算ができるように設計されています。
func ParseXMLToMap(xmlStr string) (interface{}, error) {
	decoder := xml.NewDecoder(strings.NewReader(xmlStr))
	
	// ドキュメントルートタグを最初に見つける
	for {
		t, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return nil, nil
			}
			return nil, err
		}
		
		if se, ok := t.(xml.StartElement); ok {
			val, err := parseElement(decoder, &se)
			if err != nil {
				return nil, err
			}
			// ルート要素名をキーとしたマップを返す
			return map[string]interface{}{
				se.Name.Local: val,
			}, nil
		}
	}
}

// parseElement は指定された StartElement の内部を再帰的にパースし、
// マップ、スライス、または文字列を返します。
func parseElement(dec *xml.Decoder, start *xml.StartElement) (interface{}, error) {
	children := make(map[string][]interface{})
	var textBuilder strings.Builder
	
	// 属性（Attributes）の処理
	// 属性がある場合は、マップとして記録する（プレフィックス "@" をキーに付与）
	attrs := make(map[string]interface{})
	for _, attr := range start.Attr {
		attrs[xmlAttrPrefix+attr.Name.Local] = attr.Value
	}
	
	for {
		t, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		
		switch tok := t.(type) {
		case xml.StartElement:
			// 子要素の再帰的パース
			childVal, err := parseElement(dec, &tok)
			if err != nil {
				return nil, err
			}
			tagName := tok.Name.Local
			children[tagName] = append(children[tagName], childVal)
			
		case xml.EndElement:
			// 現在の要素の終了
			if tok.Name.Local == start.Name.Local {
				return buildResult(attrs, children, textBuilder.String()), nil
			}
			
		case xml.CharData:
			// テキストデータの蓄積
			textBuilder.Write(tok)
		}
	}
	
	return buildResult(attrs, children, textBuilder.String()), nil
}

// buildResult はパースされた属性、子要素、テキストから最終的なデータ構造を決定します。
func buildResult(attrs map[string]interface{}, children map[string][]interface{}, rawText string) interface{} {
	trimmedText := strings.TrimSpace(rawText)
	
	// 子要素がなく、属性もない場合は、シンプルなテキスト（文字列）を返す
	if len(children) == 0 && len(attrs) == 0 {
		return trimmedText
	}
	
	result := make(map[string]interface{})
	
	// 属性を結果マップに追加
	for k, v := range attrs {
		result[k] = v
	}
	
	// 子要素を結果マップに追加
	for tag, list := range children {
		if len(list) == 1 {
			// 子タグが1つだけ出現した場合はオブジェクト
			result[tag] = list[0]
		} else {
			// 同名タグが複数出現した場合は配列化
			result[tag] = list
		}
	}
	
	// テキストが存在し、かつ属性や子要素もある場合は xmlTextKey で追加する
	if trimmedText != "" {
		if len(result) == 0 {
			return trimmedText
		}
		result[xmlTextKey] = trimmedText
	}
	
	return result
}
