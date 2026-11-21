package aitools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"unicode/utf8"
)

// maxSources is how many sources one answer may name.
const maxSources = 16

// sourceSchema is one source the agent names. Every key is required and the
// unused ones stay empty, so the shape suits the strict structured output of
// every agent: a git range (base, head, paths) or lines of a file (path,
// first, last).
const sourceSchema = `{"type":"object","additionalProperties":false,` +
	`"required":["id","kind","base","head","paths","path","first","last"],` +
	`"properties":{"id":{"type":"string"},"kind":{"type":"string","enum":["git","file"]},` +
	`"base":{"type":"string"},"head":{"type":"string"},"paths":{"type":"array","items":{"type":"string"}},` +
	`"path":{"type":"string"},"first":{"type":"integer"},"last":{"type":"integer"}}}`

// Wrap is the schema the agent answers with: the page's schema under "data",
// and, when the page lets the agent name sources, the list of sources.
func Wrap(page json.RawMessage, sources bool) json.RawMessage {
	var b bytes.Buffer
	b.WriteString(`{"type":"object","additionalProperties":false,`)
	if sources {
		b.WriteString(`"required":["data","sources"],"properties":{"data":`)
		b.Write(page)
		fmt.Fprintf(&b, `,"sources":{"type":"array","maxItems":%d,"items":%s}}}`, maxSources, sourceSchema)
	} else {
		b.WriteString(`"required":["data"],"properties":{"data":`)
		b.Write(page)
		b.WriteString(`}}`)
	}
	return b.Bytes()
}

// maxSchemaDepth bounds how deep Validate follows a schema, which comes from
// the page and so from anyone.
const maxSchemaDepth = 64

// Validate checks value against schema with the part of JSON Schema that
// ClayJS's schemaFor writes and that page schemas use: type, enum, const,
// properties, required, additionalProperties, items, anyOf and oneOf (as
// anyOf), and the length and range bounds. Other keywords are ignored. The
// error names where the value went wrong, as data.files[0].anchor.
func Validate(schema, value json.RawMessage) error {
	s, err := decode(schema)
	if err != nil {
		return fmt.Errorf("the answer shape is not JSON: %w", err)
	}
	v, err := decode(value)
	if err != nil {
		return fmt.Errorf("the answer is not JSON: %w", err)
	}
	return check(s, v, "the answer", 0)
}

func decode(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func check(schema, value any, at string, depth int) error {
	if depth > maxSchemaDepth {
		return fmt.Errorf("%s: the answer shape is nested too deeply", at)
	}
	switch s := schema.(type) {
	case bool:
		if !s {
			return fmt.Errorf("%s: no value is allowed here", at)
		}
		return nil
	case map[string]any:
		return checkObject(s, value, at, depth)
	}
	return nil
}

func checkObject(s map[string]any, value any, at string, depth int) error {
	if t, ok := s["type"]; ok && !typeMatches(t, value) {
		return fmt.Errorf("%s: expected %s", at, typeLabel(t))
	}
	if enum, ok := s["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if sameJSON(e, value) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: not one of the allowed values", at)
		}
	}
	if c, ok := s["const"]; ok && !sameJSON(c, value) {
		return fmt.Errorf("%s: not the required value", at)
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		options, ok := s[key].([]any)
		if !ok || len(options) == 0 {
			continue
		}
		matched := false
		for _, option := range options {
			if check(option, value, at, depth+1) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: matches none of the allowed shapes", at)
		}
	}
	switch v := value.(type) {
	case map[string]any:
		return checkFields(s, v, at, depth)
	case []any:
		if n, ok := number(s["minItems"]); ok && float64(len(v)) < n {
			return fmt.Errorf("%s: needs at least %v items", at, n)
		}
		if n, ok := number(s["maxItems"]); ok && float64(len(v)) > n {
			return fmt.Errorf("%s: allows at most %v items", at, n)
		}
		if items, ok := s["items"]; ok {
			for i, item := range v {
				if err := check(items, item, fmt.Sprintf("%s[%d]", at, i), depth+1); err != nil {
					return err
				}
			}
		}
	case string:
		length := float64(utf8.RuneCountInString(v))
		if n, ok := number(s["minLength"]); ok && length < n {
			return fmt.Errorf("%s: is shorter than %v characters", at, n)
		}
		if n, ok := number(s["maxLength"]); ok && length > n {
			return fmt.Errorf("%s: is longer than %v characters", at, n)
		}
	case json.Number:
		f, _ := v.Float64()
		if n, ok := number(s["minimum"]); ok && f < n {
			return fmt.Errorf("%s: is below %v", at, n)
		}
		if n, ok := number(s["maximum"]); ok && f > n {
			return fmt.Errorf("%s: is above %v", at, n)
		}
	}
	return nil
}

func checkFields(s map[string]any, v map[string]any, at string, depth int) error {
	if required, ok := s["required"].([]any); ok {
		for _, r := range required {
			if name, ok := r.(string); ok {
				if _, present := v[name]; !present {
					return fmt.Errorf("%s: is missing", join(at, name))
				}
			}
		}
	}
	props, _ := s["properties"].(map[string]any)
	extra, hasExtra := s["additionalProperties"]
	for name, field := range v {
		where := join(at, name)
		if p, ok := props[name]; ok {
			if err := check(p, field, where, depth+1); err != nil {
				return err
			}
			continue
		}
		if hasExtra {
			if err := check(extra, field, where, depth+1); err != nil {
				return fmt.Errorf("%s: is not part of the answer shape", where)
			}
		}
	}
	return nil
}

func typeMatches(t, value any) bool {
	switch t := t.(type) {
	case string:
		return isType(t, value)
	case []any:
		for _, one := range t {
			if name, ok := one.(string); ok && isType(name, value) {
				return true
			}
		}
		return false
	}
	return true
}

func isType(name string, value any) bool {
	switch name {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		n, ok := value.(json.Number)
		if !ok {
			return false
		}
		f, err := n.Float64()
		return err == nil && f == math.Trunc(f)
	}
	return true
}

func typeLabel(t any) string {
	switch t := t.(type) {
	case string:
		return "a value of type " + t
	case []any:
		return fmt.Sprintf("a value of one of the types %v", t)
	}
	return "another type"
}

func number(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	return f, err == nil
}

func sameJSON(a, b any) bool {
	an, aok := a.(json.Number)
	bn, bok := b.(json.Number)
	if aok && bok {
		af, _ := an.Float64()
		bf, _ := bn.Float64()
		return af == bf
	}
	return reflect.DeepEqual(a, b)
}

func join(at, name string) string {
	if at == "the answer" {
		return name
	}
	return at + "." + name
}
