package mindlink

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DecodeStrictObject keeps duplicate and null required fields from becoming Go
// zero values before authorization or an operation-specific DTO sees them.
func DecodeStrictObject(raw []byte, dst any, required []string) error {
	if len(raw) == 0 || len(raw) > MaxBodyBytes || !utf8.Valid(raw) || !validEscapes(raw) {
		return ErrInvalidRequest
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return ErrInvalidRequest
	}
	if err := scanObject(d, 0, required, reflect.TypeOf(dst)); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalidRequest
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return ErrInvalidRequest
	}
	return nil
}

func scanObject(d *json.Decoder, depth int, required []string, objectType reflect.Type) error {
	seen := make(map[string]bool)
	nulls := make(map[string]bool)
	objectType = indirectType(objectType)
	var fields map[string]reflect.Type
	if objectType != nil && objectType.Kind() == reflect.Struct {
		fields = make(map[string]reflect.Type)
		for i := 0; i < objectType.NumField(); i++ {
			field := objectType.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
	}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return ErrInvalidRequest
		}
		seen[key] = true
		var valueType reflect.Type
		if fields != nil {
			var exists bool
			valueType, exists = fields[key]
			if !exists {
				return ErrInvalidRequest
			}
		} else if objectType != nil && objectType.Kind() == reflect.Map {
			valueType = objectType.Elem()
		}
		isNull, err := scanValue(d, depth+1, valueType)
		if err != nil {
			return err
		}
		nulls[key] = isNull
	}
	last, err := d.Token()
	if err != nil || last != json.Delim('}') {
		return ErrInvalidRequest
	}
	for _, field := range required {
		if !seen[field] || nulls[field] {
			return ErrInvalidRequest
		}
	}
	return nil
}

func scanValue(d *json.Decoder, depth int, valueType reflect.Type) (bool, error) {
	if depth > 32 {
		return false, ErrInvalidRequest
	}
	token, err := d.Token()
	if err != nil {
		return false, ErrInvalidRequest
	}
	if token == nil {
		return true, nil
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			return false, scanObject(d, depth, nil, valueType)
		case '[':
			var itemType reflect.Type
			valueType = indirectType(valueType)
			if valueType != nil && (valueType.Kind() == reflect.Slice || valueType.Kind() == reflect.Array) {
				itemType = valueType.Elem()
			}
			for d.More() {
				if _, err := scanValue(d, depth+1, itemType); err != nil {
					return false, err
				}
			}
			last, err := d.Token()
			if err != nil || last != json.Delim(']') {
				return false, ErrInvalidRequest
			}
		default:
			return false, ErrInvalidRequest
		}
	}
	return false, nil
}

func indirectType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

// encoding/json replaces lone UTF-16 surrogates; reject them before that lossy
// conversion can collapse distinct signed identities or duplicate field names.
func validEscapes(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		value, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
