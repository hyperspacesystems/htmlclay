package dataguard

import (
	"github.com/panphora/htmlclay/internal/dataapi"
	"github.com/panphora/htmlclay/internal/pageadmission"
	"strings"
)

func ExtractIsland(src []byte) (dataapi.Value, error) {
	file := storedPage(string(src))
	result, declarations, err := dataapi.ReadGuardDocument([]byte(file))
	if err != nil {
		return nil, err
	}
	if !result.Found || result.Data == nil {
		return nil, nil
	}
	island := dataapi.NewObject()
	island.Set("api", result.Data)
	if declarations.Len() > 0 {
		codecs := dataapi.NewObject()
		codecs.Set("api", declarations)
		island.Set(CodecsKey, codecs)
	}
	island.Set(FormKey, "stored")
	return island, nil
}

func NamesExactVersion(header string) bool {
	header = strings.TrimSpace(header)
	if header == "" || header == "*" || strings.HasPrefix(header, "W/") || strings.Contains(header, ",") {
		return false
	}
	return true
}

func stripPageState(value dataapi.Value) dataapi.Value {
	switch v := value.(type) {
	case string:
		if !strings.Contains(v, "merge=") {
			return v
		}
		return storedPage(v)
	case []dataapi.Value:
		out := make([]dataapi.Value, len(v))
		for i, child := range v {
			out[i] = stripPageState(child)
		}
		return out
	case *dataapi.Object:
		out := dataapi.NewObject()
		for _, key := range v.Keys() {
			child, _ := v.Get(key)
			out.Define(key, stripPageState(child))
		}
		return out
	}
	return value
}

func NormalizeComposedBaseline(island dataapi.Value, previous []byte) dataapi.Value {
	obj, ok := island.(*dataapi.Object)
	if !ok || textField(obj, FormKey) == "stored" {
		return island
	}
	if !hasPageRevision(string(previous)) {
		return island
	}
	normalized := stripPageState(island).(*dataapi.Object)
	normalized.Set(FormKey, "stored")
	return normalized
}

func OrdinaryData(island dataapi.Value) dataapi.Value {
	value, _ := objectLookup(island, "api")
	obj, ok := value.(*dataapi.Object)
	if !ok {
		return value
	}
	out := dataapi.NewObject()
	for _, key := range obj.Keys() {
		if declared(island, "api", key) == "" {
			v, _ := obj.Get(key)
			out.Define(key, v)
		}
	}
	return out
}

func hasPageRevision(source string) bool {
	return pageadmission.PageRev(source) != ""
}

func storedPage(source string) string {
	if !strings.Contains(source, "merge=") || !hasPageRevision(source) {
		return source
	}
	return pageadmission.StripPageTag(source)
}

func StoredIsland(value dataapi.Value) dataapi.Value {
	obj, ok := value.(*dataapi.Object)
	if !ok {
		return value
	}
	out := dataapi.NewObject()
	for _, key := range obj.Keys() {
		v, _ := obj.Get(key)
		out.Define(key, v)
	}
	out.Set(FormKey, "stored")
	return out
}
