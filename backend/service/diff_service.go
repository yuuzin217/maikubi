package service

import (
	"encoding/json"
	"fmt"
	"maikubi/backend/model"
	"regexp"
	"sort"
	"strings"
)

var arrayIndexRegex = regexp.MustCompile(`\[\d+\]`)

type DiffService struct{}

func NewDiffService() *DiffService {
	return &DiffService{}
}

// GenerateDiffLines は、3つの環境のレスポンス（生JSON）をパースし、
// 自動ノイズ検出と手動Ignoreリストを適用した上で、フラットな DiffLine 配列を構築します。
// targets[0]: Production, targets[1]: Staging, targets[2]: Baseline を想定。
func (ds *DiffService) GenerateDiffLines(prodJSON, stagingJSON, baselineJSON string, manualIgnores []string) ([]model.DiffLine, bool, error) {
	// 1. 各環境のJSONを構造化（map または slice）にデコード
	var prod, staging, baseline interface{}
	
	// Production はオプショナル（取得失敗時はノイズ検出をスキップ）
	if prodJSON != "" {
		if err := json.Unmarshal([]byte(prodJSON), &prod); err != nil {
			prod = nil
		}
	}
	
	if err := json.Unmarshal([]byte(stagingJSON), &staging); err != nil {
		return nil, false, fmt.Errorf("staging JSON unmarshal error: %w", err)
	}
	if err := json.Unmarshal([]byte(baselineJSON), &baseline); err != nil {
		return nil, false, fmt.Errorf("baseline JSON unmarshal error: %w", err)
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
	isMatched := ds.compareAndSerialize(baseline, staging, "$", 0, noisePaths, manualIgnoreMap, &lines)

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
