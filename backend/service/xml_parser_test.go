package service

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseXMLToMap_Simple(t *testing.T) {
	xmlStr := `
	<response>
		<status>success</status>
		<code type="int">200</code>
	</response>`

	expected := map[string]interface{}{
		"response": map[string]interface{}{
			"status": "success",
			"code": map[string]interface{}{
				xmlAttrPrefix + "type": "int",
				xmlTextKey:             "200",
			},
		},
	}

	result, err := ParseXMLToMap(xmlStr)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result structure mismatch.\nExpected: %+v\nGot:      %+v", expected, result)
	}
}

func TestParseXMLToMap_ArrayAndNest(t *testing.T) {
	xmlStr := `
	<users>
		<user id="1">
			<name>Alice</name>
			<role>Admin</role>
		</user>
		<user id="2">
			<name>Bob</name>
			<role>User</role>
		</user>
		<metadata>
			<version>1.0.0</version>
		</metadata>
	</users>`

	expected := map[string]interface{}{
		"users": map[string]interface{}{
			"user": []interface{}{
				map[string]interface{}{
					xmlAttrPrefix + "id":  "1",
					"name":                "Alice",
					"role":                "Admin",
				},
				map[string]interface{}{
					xmlAttrPrefix + "id":  "2",
					"name":                "Bob",
					"role":                "User",
				},
			},
			"metadata": map[string]interface{}{
				"version": "1.0.0",
			},
		},
	}

	result, err := ParseXMLToMap(xmlStr)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result structure mismatch.\nExpected: %+v\nGot:      %+v", expected, result)
	}
}

func TestParseXMLToMap_EmptyAndEdgeCases(t *testing.T) {
	// 空のタグや、テキストノード単体、属性なしのリーフ要素などを検証
	xmlStr := `<root><empty></empty><selfClosing/><textOnly>Hello</textOnly></root>`

	expected := map[string]interface{}{
		"root": map[string]interface{}{
			"empty":       "",
			"selfClosing": "",
			"textOnly":    "Hello",
		},
	}

	result, err := ParseXMLToMap(xmlStr)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Result structure mismatch.\nExpected: %+v\nGot:      %+v", expected, result)
	}
}

func TestGenerateDiffLines_XML_TagOutput(t *testing.T) {
	prodXML := `<response><id>100</id><name>Alice</name></response>`
	stagingXML := `<response><id>100</id><name>Alice Pro</name><email>alice@example.com</email></response>`
	baselineXML := `<response><id>100</id><name>Alice</name></response>`

	ds := NewDiffService()
	lines, isMatched, err := ds.GenerateDiffLines(prodXML, stagingXML, baselineXML, nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if isMatched {
		t.Error("Expected isMatched to be false")
	}

	// 出力行が本物のXMLタグ形式になっているか検証
	var foundStartTag, foundEndTag, foundEmailAdded bool
	for _, l := range lines {
		t.Logf("Line: %s (Status: %s, Path: %s)", l.Text, l.Status, l.JSONPath)
		if strings.Contains(l.Text, "<response>") {
			foundStartTag = true
		}
		if strings.Contains(l.Text, "</response>") {
			foundEndTag = true
		}
		if strings.Contains(l.Text, "<email>alice@example.com</email>") && l.Status == "added" {
			foundEmailAdded = true
		}
	}

	if !foundStartTag {
		t.Error("Expected to find '<response>' start tag in diff output")
	}
	if !foundEndTag {
		t.Error("Expected to find '</response>' end tag in diff output")
	}
	if !foundEmailAdded {
		t.Error("Expected to find added '<email>' tag in diff output")
	}
}
