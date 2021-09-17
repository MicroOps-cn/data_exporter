package collector

import (
	"bytes"
	"fmt"
	"github.com/tidwall/gjson"
	"strconv"
	"strings"
)

func splitOnce(s string, sep byte) (string, string) {
	var prefix, suffix = s, ""
	idx := strings.IndexByte(s, sep)
	if idx >= 0 {
		prefix = s[:idx]
		suffix = s[idx+1:]
	}
	return prefix, suffix
}

func toEntries(jsonStr, arg string) string {
	var keyName, valName = splitOnce(arg, ':')
	if keyName == valName {
		keyName, valName = "key", "value"
	} else if keyName == "" {
		keyName = "key"
	} else if valName == "" {
		valName = "value"
	}
	o := gjson.Parse(jsonStr)
	if o.IsObject() {
		var buf = bytes.NewBuffer([]byte{'['})
		var isFirstLine = true
		for key, value := range o.Map() {
			if isFirstLine {
				isFirstLine = false
			} else {
				buf.WriteString(",")
			}
			if keyName == "-" {
				buf.WriteString(value.Raw)
			} else if valName == "-" {
				buf.WriteString(fmt.Sprintf(`"%s"`, key))
			} else {
				buf.WriteString(fmt.Sprintf(`{"%s": "%s", "%s": %s}`, keyName, key, valName, value.Raw))
			}
		}
		buf.WriteString("]")
		jsonStr = buf.String()
	}
	return jsonStr
}

func appendPath(r gjson.Result, path string) gjson.Result {
	buf := bytes.Buffer{}
	if r.IsArray() {
		buf.WriteByte('[')
		for idx, result := range r.Array() {
			if idx != 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(result.Raw)
		}
		buf.WriteByte(',')
		buf.WriteString("," + path + "]")
		return gjson.Parse(buf.String())
	} else {
		return newGJsonString(r.String() + "." + path)
	}
}
func newGJsonString(s string) gjson.Result {
	return gjson.Result{Type: gjson.String, Raw: fmt.Sprintf(`"%s"`, s), Str: s}
}

type GMap map[string]gjson.Result

func (G GMap) String() string {
	var buf = bytes.NewBuffer([]byte{'{'})
	var idx = 0
	for k2, v2 := range G {
		if idx != 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(fmt.Sprintf(`"%s": %s`, k2, v2.Raw))
		idx++
	}
	buf.WriteByte('}')
	return buf.String()
}

type GArray []gjson.Result

func (G GArray) String() string {
	var buf = bytes.NewBuffer([]byte{'['})
	for idx, v := range G {
		if idx != 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(v.Raw)
		idx++
	}
	return buf.String()
}

func drillDown(jsonStr string, arg string) string {
	var pathName = "path"
	if arg != "" {
		pathName = arg
	}
	o := gjson.Parse(jsonStr)

	var buf = bytes.NewBuffer([]byte{'['})
	if o.IsObject() {
		var objMap GMap = o.Map()
		var idx = 0
		for key, value := range objMap {
			if value.IsObject() {
				var valMap GMap = value.Map()
				if v, ok := valMap[pathName]; ok {
					valMap[pathName] = appendPath(v, key)
				} else {
					valMap[pathName] = newGJsonString(key)
				}
				if idx != 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(valMap.String())
				idx++
			}
		}
	} else if o.IsArray() {
		var objArray GArray = o.Array()
		var idx = 0
		for i, value := range objArray {
			if value.IsObject() {
				var valMap GMap = value.Map()
				if v, ok := valMap[pathName]; ok {
					valMap[pathName] = appendPath(v, strconv.Itoa(i))
				} else {
					valMap[pathName] = newGJsonString(strconv.Itoa(i))
				}
				if idx != 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(valMap.String())
				idx++
			}
		}
	}
	buf.WriteByte(']')
	return buf.String()
}

func expand(jsonStr string, _ string) string {
	o := gjson.Parse(jsonStr)

	var buf = bytes.NewBuffer([]byte{'{'})
	if o.IsObject() {
		var objMap GMap = o.Map()
		var idx = 0
		for key, value := range objMap {
			if value.IsObject() {
				var valMap GMap = value.Map()
				for s, result := range valMap {
					if idx != 0 {
						buf.WriteByte(',')
					}
					buf.WriteString(fmt.Sprintf(`"%s.%s":%s`, key, s, result.Raw))
					idx++
				}
			}
		}
	} else if o.IsArray() {
		var objArray GArray = o.Array()
		var idx = 0
		for i, value := range objArray {
			if value.IsObject() {
				var valMap GMap = value.Map()
				for s, result := range valMap {
					if idx != 0 {
						buf.WriteByte(',')
					}
					buf.WriteString(fmt.Sprintf(`"%d.%s":%s'`, i, s, result.Raw))
					idx++
				}
			}
		}
	}
	buf.WriteByte('}')
	return buf.String()
}

func init() {
	gjson.AddModifier("to_entries", toEntries)
	gjson.AddModifier("drill_down", drillDown)
	gjson.AddModifier("expand", expand)
}
