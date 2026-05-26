package service

import (
	"encoding/json"
	"fmt"
	"maikubi/backend/model"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var arrayIndexRegex = regexp.MustCompile(`\[\d+\]`)

type DiffService struct{}

func NewDiffService() *DiffService {
	return &DiffService{}
}

// isXML は文字列がXML形式かどうかを簡易判定します。
func isXML(str string) bool {
	trimmed := strings.TrimSpace(str)
	return strings.HasPrefix(trimmed, "<")
}

// parseRawResponse は、文字列がXMLであればXMLパーサーを、そうでなければJSONデコーダーを用いて構造化データを返します。
func (ds *DiffService) parseRawResponse(str string) (interface{}, error) {
	if str == "" {
		return nil, nil
	}
	if isXML(str) {
		return ParseXMLToMap(str)
	}
	var val interface{}
	if err := json.Unmarshal([]byte(str), &val); err != nil {
		return nil, err
	}
	return val, nil
}

// GenerateDiffLines は、3つの環境のレスポンス（生JSON/XML）をパースし、
// 自動ノイズ検出と手動Ignoreリストを適用した上で、フラットな DiffLine 配列を構築します。
// targets[0]: Production, targets[1]: Staging, targets[2]: Baseline を想定。
func (ds *DiffService) GenerateDiffLines(prodJSON, stagingJSON, baselineJSON string, manualIgnores []string) ([]model.DiffLine, bool, error) {
	// 1. 各環境のデータを構造化（map または slice）にデコード（XML/JSON自動判定）
	var prod, staging, baseline interface{}
	var err error
	
	if prodJSON != "" {
		prod, _ = ds.parseRawResponse(prodJSON)
	}
	
	staging, err = ds.parseRawResponse(stagingJSON)
	if err != nil {
		return nil, false, fmt.Errorf("staging parse error: %w", err)
	}
	baseline, err = ds.parseRawResponse(baselineJSON)
	if err != nil {
		return nil, false, fmt.Errorf("baseline parse error: %w", err)
	}

	// 2. 自動ノイズパスの検出 (Production vs Baseline)
	noisePaths := make(map[string]bool)
	if prod != nil && baseline != nil {
		ds.detectNoise(prod, baseline, "$", noisePaths)
	}

	// 3. 手動Ignoreリストのマップ化
	manualIgnoreMap := make(map[string]bool)
	for _, p := range manualIgnores {
		manualIgnoreMap[p] = true
	}

	// 4. Staging vs Baseline の構造的マージ比較 ＆ シリアライズ
	var lines []model.DiffLine
	var isMatched bool

	if isXML(stagingJSON) {
		// XML の場合：ルートタグ名を抽出してセマンティック比較
		rootTag := "response"
		if rootMap, ok := staging.(map[string]interface{}); ok {
			for k := range rootMap {
				rootTag = k
				break
			}
			isMatched = ds.compareAndSerializeXML(
				baseline.(map[string]interface{})[rootTag],
				staging.(map[string]interface{})[rootTag],
				rootTag,
				"$."+rootTag,
				0,
				noisePaths,
				manualIgnoreMap,
				&lines,
			)
		} else {
			isMatched = ds.compareAndSerializeXML(baseline, staging, "root", "$", 0, noisePaths, manualIgnoreMap, &lines)
		}
	} else {
		// JSON の場合
		isMatched = ds.compareAndSerialize(baseline, staging, "$", 0, noisePaths, manualIgnoreMap, &lines)
	}

	// 行番号の付与
	for i := range lines {
		lines[i].LineNumber = i + 1
	}

	return lines, isMatched, nil
}

// detectNoise は、PとBを比較して値が異なる JSONPath を自動検出します。
func (ds *DiffService) detectNoise(p, b interface{}, path string, noisePaths map[string]bool) {
	if fmt.Sprintf("%T", p) != fmt.Sprintf("%T", b) {
		noisePaths[path] = true
		return
	}

	switch pVal := p.(type) {
	case map[string]interface{}:
		bMap := b.(map[string]interface{})
		for k, v := range pVal {
			subPath := fmt.Sprintf("%s.%s", path, k)
			if bv, exists := bMap[k]; exists {
				ds.detectNoise(v, bv, subPath, noisePaths)
			} else {
				noisePaths[subPath] = true
			}
		}
	case []interface{}:
		bSlice := b.([]interface{})
		if len(pVal) != len(bSlice) {
			noisePaths[path] = true
			return
		}
		for i, v := range pVal {
			subPath := fmt.Sprintf("%s[%d]", path, i)
			ds.detectNoise(v, bSlice[i], subPath, noisePaths)
		}
	default:
		if p != b {
			noisePaths[path] = true
		}
	}
}

// compareAndSerialize は、B(Baseline) と S(Staging) をマージしながらシリアライズし、DiffLine配列を作成します。
func (ds *DiffService) compareAndSerialize(
	b, s interface{},
	path string,
	indent int,
	noisePaths map[string]bool,
	manualIgnores map[string]bool,
	lines *[]model.DiffLine,
) bool {
	isIgnored := noisePaths[path] || manualIgnores[path] || ds.isWildcardIgnored(path, manualIgnores)

	if isIgnored {
		ds.serializeMatched(b, path, indent, lines)
		return true // 除外パスは「一致」扱い
	}

	// 型不一致の場合は modified (削除+追加で表現)
	if fmt.Sprintf("%T", b) != fmt.Sprintf("%T", s) {
		ds.serializeDiff(b, s, path, indent, lines)
		return false
	}

	switch bVal := b.(type) {
	case map[string]interface{}:
		sMap := s.(map[string]interface{})
		keySet := make(map[string]bool)
		for k := range bVal {
			keySet[k] = true
		}
		for k := range sMap {
			keySet[k] = true
		}
		var keys []string
		for k := range keySet {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		ds.appendLine("{", path, "matched", indent, lines)
		allChildrenMatched := true

		for i, k := range keys {
			subPath := fmt.Sprintf("%s.%s", path, k)
			bv, inB := bVal[k]
			sv, inS := sMap[k]
			suffix := ","
			if i == len(keys)-1 {
				suffix = ""
			}

			if inB && inS {
				// 再帰比較。キー名を付与したシリアライズにするため、一時的に行を取得して整形
				var childLines []model.DiffLine
				childMatched := ds.compareAndSerialize(bv, sv, subPath, indent+1, noisePaths, manualIgnores, &childLines)
				if !childMatched {
					allChildrenMatched = false
				}
				ds.injectKeyToChildLines(k, suffix, childLines, lines)
			} else if inB {
				// Baselineにしかない (Stagingで削除)
				ds.serializeDeletedWithKey(bv, k, suffix, subPath, indent+1, lines)
				allChildrenMatched = false
			} else {
				// Stagingにしかない (Stagingで追加)
				ds.serializeAddedWithKey(sv, k, suffix, subPath, indent+1, lines)
				allChildrenMatched = false
			}
		}
		ds.appendLine("}", path, "matched", indent, lines)
		return allChildrenMatched

	case []interface{}:
		sSlice := s.([]interface{})
		ds.appendLine("[", path, "matched", indent, lines)
		allChildrenMatched := true

		maxLen := len(bVal)
		if len(sSlice) > maxLen {
			maxLen = len(sSlice)
		}

		for i := 0; i < maxLen; i++ {
			subPath := fmt.Sprintf("%s[%d]", path, i)
			suffix := ","
			if i == maxLen-1 {
				suffix = ""
			}

			if i < len(bVal) && i < len(sSlice) {
				var childLines []model.DiffLine
				childMatched := ds.compareAndSerialize(bVal[i], sSlice[i], subPath, indent+1, noisePaths, manualIgnores, &childLines)
				if !childMatched {
					allChildrenMatched = false
				}
				ds.injectSuffixToLastLine(suffix, childLines)
				*lines = append(*lines, childLines...)
			} else if i < len(bVal) {
				ds.serializeDeletedWithKey(bVal[i], "", suffix, subPath, indent+1, lines)
				allChildrenMatched = false
			} else {
				ds.serializeAddedWithKey(sSlice[i], "", suffix, subPath, indent+1, lines)
				allChildrenMatched = false
			}
		}
		ds.appendLine("]", path, "matched", indent, lines)
		return allChildrenMatched

	default:
		if b == s {
			ds.serializeMatched(b, path, indent, lines)
			return true
		}
		ds.serializeDiff(b, s, path, indent, lines)
		return false
	}
}

// isWildcardIgnored は、配列インデックスを[*]に変換したパスがIgnoreリストにあるか確認します。
func (ds *DiffService) isWildcardIgnored(path string, manualIgnores map[string]bool) bool {
	normalized := arrayIndexRegex.ReplaceAllString(path, "[*]")
	return manualIgnores[normalized]
}

// serializeMatched は、値をすべて matched として出力します。
func (ds *DiffService) serializeMatched(val interface{}, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeValue(val, "", "", "matched", path, indent, lines)
}

// serializeDiff は、変更差分を deleted と added のペアで出力します。
func (ds *DiffService) serializeDiff(bVal, sVal interface{}, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeValue(bVal, "", "", "deleted", path, indent, lines)
	ds.serializeValue(sVal, "", "", "added", path, indent, lines)
}

// serializeDeletedWithKey は、削除されたキー付きオブジェクトを出力します。
func (ds *DiffService) serializeDeletedWithKey(val interface{}, key, suffix, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeValue(val, key, suffix, "deleted", path, indent, lines)
}

// serializeAddedWithKey は、追加されたキー付きオブジェクトを出力します。
func (ds *DiffService) serializeAddedWithKey(val interface{}, key, suffix, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeValue(val, key, suffix, "added", path, indent, lines)
}

// serializeValue は、オブジェクト・配列・プリミティブを整形して追加します。
func (ds *DiffService) serializeValue(val interface{}, key, suffix, status, path string, indent int, lines *[]model.DiffLine) {
	prefix := ""
	if key != "" {
		prefix = fmt.Sprintf("\"%s\": ", key)
	}

	switch v := val.(type) {
	case map[string]interface{}:
		ds.appendLine(prefix+"{", path, status, indent, lines)
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			subPath := fmt.Sprintf("%s.%s", path, k)
			subSuffix := ","
			if i == len(keys)-1 {
				subSuffix = ""
			}
			ds.serializeValue(v[k], k, subSuffix, status, subPath, indent+1, lines)
		}
		ds.appendLine("}"+suffix, path, status, indent, lines)

	case []interface{}:
		ds.appendLine(prefix+"[", path, status, indent, lines)
		for i, item := range v {
			subPath := fmt.Sprintf("%s[%d]", path, i)
			subSuffix := ","
			if i == len(v)-1 {
				subSuffix = ""
			}
			ds.serializeValue(item, "", subSuffix, status, subPath, indent+1, lines)
		}
		ds.appendLine("]"+suffix, path, status, indent, lines)

	default:
		strVal := ds.formatPrimitive(val)
		ds.appendLine(prefix+strVal+suffix, path, status, indent, lines)
	}
}

// formatPrimitive は、プリミティブ値をJSON形式にフォーマットします。
func (ds *DiffService) formatPrimitive(val interface{}) string {
	b, err := json.Marshal(val)
	if err != nil {
		return fmt.Sprintf("%v", val)
	}
	return string(b)
}

// injectKeyToChildLines は、再帰で生成した子要素の最初の行に "key": を付与します。
// 値の変更 (modified) が発生した場合、削除行 (1行目) と追加行 (2行目) の両方に "key": を注入します。
func (ds *DiffService) injectKeyToChildLines(key, suffix string, childLines []model.DiffLine, dest *[]model.DiffLine) {
	if len(childLines) == 0 {
		return
	}

	// 最初の行（deleted または matched）に "key": を注入する
	firstLine := childLines[0]
	trimmedText := strings.TrimLeft(firstLine.Text, " ")
	indentLen := len(firstLine.Text) - len(trimmedText)
	indentStr := firstLine.Text[:indentLen]

	firstLine.Text = fmt.Sprintf("%s\"%s\": %s", indentStr, key, trimmedText)
	childLines[0] = firstLine

	// 値の変更 (modified) ペアの場合、2行目 (added) にも "key": を注入する
	if len(childLines) == 2 && childLines[0].Status == "deleted" && childLines[1].Status == "added" && childLines[0].JSONPath == childLines[1].JSONPath {
		secondLine := childLines[1]
		trimmedSecondText := strings.TrimLeft(secondLine.Text, " ")
		secondIndentLen := len(secondLine.Text) - len(trimmedSecondText)
		secondIndentStr := secondLine.Text[:secondIndentLen]

		secondLine.Text = fmt.Sprintf("%s\"%s\": %s", secondIndentStr, key, trimmedSecondText)
		childLines[1] = secondLine
	}

	// 最後の行にカンマなどの suffix を付加する
	ds.injectSuffixToLastLine(suffix, childLines)

	*dest = append(*dest, childLines...)
}

// injectSuffixToLastLine は、最後の行の末尾にカンマなどの接尾辞を追加します。
func (ds *DiffService) injectSuffixToLastLine(suffix string, lines []model.DiffLine) {
	if len(lines) == 0 || suffix == "" {
		return
	}
	lastIdx := len(lines) - 1
	lines[lastIdx].Text = lines[lastIdx].Text + suffix
}

// appendLine は、インデント付きの行を lines に追加します。
func (ds *DiffService) appendLine(text, path, status string, indent int, lines *[]model.DiffLine) {
	indentStr := strings.Repeat("  ", indent)
	*lines = append(*lines, model.DiffLine{
		Status:   status,
		Text:     indentStr + text,
		JSONPath: path,
	})
}

// compareAndSerializeXML は、Baseline と Staging をマージ比較しながら、XMLタグ形式の DiffLine 配列を構築します。
func (ds *DiffService) compareAndSerializeXML(
	b, s interface{},
	tag string,
	path string,
	indent int,
	noisePaths map[string]bool,
	manualIgnores map[string]bool,
	lines *[]model.DiffLine,
) bool {
	isIgnored := noisePaths[path] || manualIgnores[path] || ds.isWildcardIgnored(path, manualIgnores)

	if isIgnored {
		ds.serializeMatchedXML(b, tag, path, indent, lines)
		return true
	}

	// 型不一致の場合は modified (削除+追加)
	if fmt.Sprintf("%T", b) != fmt.Sprintf("%T", s) {
		ds.serializeDiffXML(b, s, tag, path, indent, lines)
		return false
	}

	switch bVal := b.(type) {
	case map[string]interface{}:
		sMap := s.(map[string]interface{})
		
		bAttrs, bChildren, bText := ds.classifyXMLNode(bVal)
		sAttrs, sChildren, sText := ds.classifyXMLNode(sMap)

		attrsMatched := reflect.DeepEqual(bAttrs, sAttrs)

		keySet := make(map[string]bool)
		for k := range bChildren {
			keySet[k] = true
		}
		for k := range sChildren {
			keySet[k] = true
		}
		var keys []string
		for k := range keySet {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		textMatched := bText == sText

		startTagMatched := attrsMatched
		
		if startTagMatched {
			ds.appendLine(ds.buildXMLStartTag(tag, bAttrs), path, "matched", indent, lines)
		} else {
			ds.appendLine(ds.buildXMLStartTag(tag, bAttrs), path, "deleted", indent, lines)
			ds.appendLine(ds.buildXMLStartTag(tag, sAttrs), path, "added", indent, lines)
		}

		allChildrenMatched := attrsMatched && textMatched

		for _, k := range keys {
			subPath := fmt.Sprintf("%s.%s", path, k)
			bList, _ := bChildren[k]
			sList, _ := sChildren[k]

			maxLen := len(bList)
			if len(sList) > maxLen {
				maxLen = len(sList)
			}

			for i := 0; i < maxLen; i++ {
				itemPath := subPath
				if maxLen > 1 {
					itemPath = fmt.Sprintf("%s[%d]", subPath, i)
				}

				if i < len(bList) && i < len(sList) {
					childMatched := ds.compareAndSerializeXML(bList[i], sList[i], k, itemPath, indent+1, noisePaths, manualIgnores, lines)
					if !childMatched {
						allChildrenMatched = false
					}
				} else if i < len(bList) {
					ds.serializeDeletedXML(bList[i], k, itemPath, indent+1, lines)
					allChildrenMatched = false
				} else {
					ds.serializeAddedXML(sList[i], k, itemPath, indent+1, lines)
					allChildrenMatched = false
				}
			}
		}

		if bText != "" || sText != "" {
			subPath := path + "." + xmlTextKey
			if bText == sText {
				ds.appendLine(bText, subPath, "matched", indent+1, lines)
			} else {
				allChildrenMatched = false
				if bText != "" {
					ds.appendLine(bText, subPath, "deleted", indent+1, lines)
				}
				if sText != "" {
					ds.appendLine(sText, subPath, "added", indent+1, lines)
				}
			}
		}

		ds.appendLine(fmt.Sprintf("</%s>", tag), path, "matched", indent, lines)
		return allChildrenMatched

	default:
		if b == s {
			ds.serializeMatchedXML(b, tag, path, indent, lines)
			return true
		}
		ds.serializeDiffXML(b, s, tag, path, indent, lines)
		return false
	}
}

// serializeXMLValue は、オブジェクト・配列・プリミティブをXMLタグ形式で整形して追加します。
func (ds *DiffService) serializeXMLValue(val interface{}, tag, status, path string, indent int, lines *[]model.DiffLine) {
	if tag == "" {
		tag = "element"
	}

	switch v := val.(type) {
	case map[string]interface{}:
		attrs, childElements, textVal := ds.classifyXMLNode(v)
		startTag := ds.buildXMLStartTag(tag, attrs)

		if len(childElements) == 0 && textVal == "" {
			ds.appendLine(fmt.Sprintf("<%s/>", tag), path, status, indent, lines)
			return
		}

		if len(childElements) == 0 {
			ds.appendLine(fmt.Sprintf("%s%s</%s>", startTag, textVal, tag), path, status, indent, lines)
			return
		}

		ds.appendLine(startTag, path, status, indent, lines)
		
		var keys []string
		for k := range childElements {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			subPath := fmt.Sprintf("%s.%s", path, k)
			subList := childElements[k]
			for i, subVal := range subList {
				itemPath := subPath
				if len(subList) > 1 {
					itemPath = fmt.Sprintf("%s[%d]", subPath, i)
				}
				ds.serializeXMLValue(subVal, k, status, itemPath, indent+1, lines)
			}
		}

		if textVal != "" {
			ds.appendLine(textVal, path+"."+xmlTextKey, status, indent+1, lines)
		}

		ds.appendLine(fmt.Sprintf("</%s>", tag), path, status, indent, lines)

	case []interface{}:
		for i, item := range v {
			subPath := fmt.Sprintf("%s[%d]", path, i)
			ds.serializeXMLValue(item, tag, status, subPath, indent, lines)
		}

	default:
		var strVal string
		if sVal, ok := val.(string); ok {
			strVal = sVal
		} else {
			strVal = ds.formatPrimitive(val)
		}
		ds.appendLine(fmt.Sprintf("<%s>%s</%s>", tag, strVal, tag), path, status, indent, lines)
	}
}

// classifyXMLNode はマップを属性、テキスト、および子要素に分類します。
func (ds *DiffService) classifyXMLNode(m map[string]interface{}) (map[string]string, map[string][]interface{}, string) {
	attrs := make(map[string]string)
	children := make(map[string][]interface{})
	var textVal string

	for k, v := range m {
		if strings.HasPrefix(k, xmlAttrPrefix) {
			attrs[k[len(xmlAttrPrefix):]] = fmt.Sprintf("%v", v)
		} else if k == xmlTextKey {
			textVal = fmt.Sprintf("%v", v)
		} else {
			if slice, ok := v.([]interface{}); ok {
				children[k] = slice
			} else {
				children[k] = []interface{}{v}
			}
		}
	}

	return attrs, children, textVal
}

// buildXMLStartTag はタグ名と属性マップから開始タグを作ります。
func (ds *DiffService) buildXMLStartTag(tag string, attrs map[string]string) string {
	if len(attrs) == 0 {
		return fmt.Sprintf("<%s>", tag)
	}

	var keys []string
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("<")
	sb.WriteString(tag)
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf(" %s=\"%s\"", k, attrs[k]))
	}
	sb.WriteString(">")
	return sb.String()
}

func (ds *DiffService) serializeMatchedXML(val interface{}, tag, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeXMLValue(val, tag, "matched", path, indent, lines)
}

func (ds *DiffService) serializeDeletedXML(val interface{}, tag, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeXMLValue(val, tag, "deleted", path, indent, lines)
}

func (ds *DiffService) serializeAddedXML(val interface{}, tag, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeXMLValue(val, tag, "added", path, indent, lines)
}

func (ds *DiffService) serializeDiffXML(bVal, sVal interface{}, tag, path string, indent int, lines *[]model.DiffLine) {
	ds.serializeXMLValue(bVal, tag, "deleted", path, indent, lines)
	ds.serializeXMLValue(sVal, tag, "added", path, indent, lines)
}
