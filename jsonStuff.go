package subaccounting

import (
	"encoding/json"
	"fmt"
	"runtime"
)

// JSONObject simple structure for utilizing a JSON Object
type JSONObject map[string]interface{}

// JSONObjectArray simple structure for utilizing a JSON Array of Objects
type JSONObjectArray []JSONObject

// Merge takes a JSONObject and does a shallow merge with another one
func (obj *JSONObject) Merge(s2 JSONObject) {
	tmp := *obj

	for k, v := range s2 {
		tmp[k] = v
	}

	*obj = tmp
}

// String with the key, returns a string value of it
func (obj *JSONObject) String(key string) string {
	if tmp, ok := (*obj)[key]; ok {
		return fmt.Sprintf("%v", tmp)
	}
	return ""
}

// Copy takes a JSONObject and creates an identical one at a new memory address
func (obj *JSONObject) Copy() JSONObject {
	tmp := obj.Export()
	tmp2 := JSONObject{}
	tmp2.ImportRaw(tmp)
	return tmp2
}

// Export takes the JSONObject and converts it back to raw JSON
func (obj *JSONObject) Export() json.RawMessage {
	str, _ := json.Marshal(obj)
	return str
}

// ImportRaw takes the raw JSON and overwrites the existing structure with it.
func (obj *JSONObject) ImportRaw(str json.RawMessage) {
	err := json.Unmarshal(str, &obj)
	if err != nil {
		describe(err)
	}
}

// ImportString takes the string version of JSON and overwrites the existing structure with it.
func (obj *JSONObject) ImportString(str string) {
	obj.ImportRaw(json.RawMessage(str))
}

// Export takes the JSONObjectArray and converts it back to raw JSON
func (obj *JSONObjectArray) Export() json.RawMessage {
	str, _ := json.Marshal(obj)
	return str
}

// ImportRaw takes the raw JSON and overwrites the existing structure with it.
func (obj *JSONObjectArray) ImportRaw(str json.RawMessage) {
	o := *obj
	tmp := [](map[string]interface{}){}
	o = JSONObjectArray{}
	err := json.Unmarshal(str, &tmp)
	if err != nil {
		describe(err)
	}
	for i := 0; i < len(tmp); i++ {
		o = append(o, JSONObject(tmp[i]))
	}
	*obj = o
}

// ImportString takes the string version of JSON and overwrites the existing structure with it.
func (obj *JSONObjectArray) ImportString(str string) {
	obj.ImportRaw(json.RawMessage(str))
}

func describe(i interface{}) {
	pc, fn, line, _ := runtime.Caller(1)
	fmt.Printf("----  %s[%s:%d]\n---- (%+v, %T)\n\n", runtime.FuncForPC(pc).Name(), fn, line, i, i)
}
