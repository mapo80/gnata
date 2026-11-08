// Package rawjson reads values inside a JSON document without decoding the
// document: a Result is one value, with its raw text, its type and, for
// strings and numbers, its scalar. It reads with encoding/json/jsontext from
// the standard library and keeps the semantics the byte fast paths had with
// github.com/tidwall/gjson: the first of duplicate object names is the one
// found, a numeric step indexes an array, string contents keep invalid UTF-8
// as it is when they have no escape, and a document is read only as far as
// a lookup needs (bytes after the value looked for are not examined).
//
// A Result's Raw and Str are copies, safe to keep; its document bytes are
// the caller's and are read only during the calls that receive them.
package rawjson

import (
	"bytes"
	"encoding/json/jsontext"
	"strconv"
	"strings"
	"sync"
)

// Type is the JSON type of a Result.
type Type int

const (
	// Null is JSON null, or the type of a Result that does not exist.
	Null Type = iota
	// False is JSON false.
	False
	// Number is a JSON number.
	Number
	// String is a JSON string.
	String
	// True is JSON true.
	True
	// JSON is an object or an array.
	JSON
)

// Result is one JSON value of a document.
type Result struct {
	// Type is the JSON type.
	Type Type
	// Raw is the value's text. For the object or array that Parse returns,
	// it runs to the end of the document.
	Raw string
	// Str is the content of a string.
	Str string
	// Num is the value of a number, as strconv.ParseFloat reads Raw.
	Num float64
	// data holds the bytes of Raw in the caller's document.
	data []byte
}

// lenient reads documents as the gjson fast paths did: duplicate names and
// invalid UTF-8 are accepted.
var lenient = []jsontext.Options{jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true)}

// decoder reads the bytes held by buf directly (jsontext does not copy a
// bytes.Buffer); both are reused across calls.
type decoder struct {
	buf bytes.Buffer
	dec jsontext.Decoder
}

var decoders = sync.Pool{New: func() any { return new(decoder) }}

func getDecoder(data []byte) *decoder {
	d := decoders.Get().(*decoder)
	d.buf = *bytes.NewBuffer(data)
	d.dec.Reset(&d.buf, lenient...)
	return d
}

func putDecoder(d *decoder) {
	d.buf = bytes.Buffer{}
	d.dec.Reset(&d.buf)
	decoders.Put(d)
}

// Parse returns the value that starts data. An object or an array is not
// read here: ForEach reads its members when asked. A scalar is read whole.
// The Result does not exist when data holds no value.
func Parse(data []byte) Result {
	data = bytes.TrimLeft(data, " \t\r\n")
	if len(data) == 0 {
		return Result{}
	}
	if data[0] == '{' || data[0] == '[' {
		return Result{Type: JSON, Raw: string(data), data: data}
	}
	d := getDecoder(data)
	defer putDecoder(d)
	value, err := d.dec.ReadValue()
	if err != nil {
		return Result{}
	}
	return newResult(value, string(value))
}

// Get follows steps from the value that starts data, reading data once and
// only as far as needed: a step names the first member of that name in an
// object or, when it is a non-negative decimal index, the element of an
// array. The Result does not exist when a step finds nothing or the bytes
// read are not valid JSON.
func Get(data []byte, steps []string) Result {
	d := getDecoder(data)
	defer putDecoder(d)
	dec := &d.dec
	for _, step := range steps {
		switch dec.PeekKind() {
		case '{':
			if _, err := dec.ReadToken(); err != nil {
				return Result{}
			}
			for {
				if dec.PeekKind() != '"' {
					return Result{}
				}
				name, err := dec.ReadValue()
				if err != nil {
					return Result{}
				}
				if nameIs(name, step) {
					break
				}
				if err := dec.SkipValue(); err != nil {
					return Result{}
				}
			}
		case '[':
			index, err := strconv.Atoi(step)
			if err != nil || index < 0 || strconv.Itoa(index) != step {
				return Result{}
			}
			if _, err := dec.ReadToken(); err != nil {
				return Result{}
			}
			for ; index > 0; index-- {
				if k := dec.PeekKind(); k == ']' || k == 0 {
					return Result{}
				}
				if err := dec.SkipValue(); err != nil {
					return Result{}
				}
			}
			if k := dec.PeekKind(); k == ']' || k == 0 {
				return Result{}
			}
		default:
			return Result{}
		}
	}
	value, err := dec.ReadValue()
	if err != nil {
		return Result{}
	}
	return newResult(value, string(value))
}

// nameIs reports whether the quoted object name raw is name.
func nameIs(raw []byte, name string) bool {
	if bytes.IndexByte(raw, '\\') < 0 {
		return string(raw[1:len(raw)-1]) == name
	}
	out, _ := jsontext.AppendUnquote(nil, raw)
	return string(out) == name
}

// newResult makes the Result of a whole value: data in the caller's
// document, raw its text in memory the Result may keep.
func newResult(data []byte, raw string) Result {
	r := Result{Raw: raw, data: data}
	switch data[0] {
	case 'n':
		r.Type = Null
	case 'f':
		r.Type = False
	case 't':
		r.Type = True
	case '"':
		r.Type = String
		r.Str = unquote(raw)
	case '{', '[':
		r.Type = JSON
	default:
		r.Type = Number
		r.Num, _ = strconv.ParseFloat(raw, 64)
	}
	return r
}

// unquote returns the content of a JSON string: a substring of raw when it
// has no escape, otherwise the unescaped text.
func unquote(raw string) string {
	if strings.IndexByte(raw, '\\') < 0 {
		return raw[1 : len(raw)-1]
	}
	out, _ := jsontext.AppendUnquote(nil, raw)
	return string(out)
}

// Bytes returns the value's bytes in the caller's document (for the object
// or array that Parse returns, up to the end of the document).
func (r Result) Bytes() []byte {
	return r.data
}

// Exists reports whether the Result is a value of the document.
func (r Result) Exists() bool {
	return r.Type != Null || len(r.Raw) != 0
}

// IsObject reports whether the Result is a JSON object.
func (r Result) IsObject() bool {
	return r.Type == JSON && len(r.Raw) > 0 && r.Raw[0] == '{'
}

// IsArray reports whether the Result is a JSON array.
func (r Result) IsArray() bool {
	return r.Type == JSON && len(r.Raw) > 0 && r.Raw[0] == '['
}

// Float returns the value of a number, zero for any other type.
func (r Result) Float() float64 {
	if r.Type == Number {
		return r.Num
	}
	return 0
}

// String returns the content of a string and the raw text of any other
// value ("" for a Result that does not exist).
func (r Result) String() string {
	if r.Type == String {
		return r.Str
	}
	return r.Raw
}

// Array returns the elements of an array; a value that is not an array is
// its only element, and a Result that does not exist has none.
func (r Result) Array() []Result {
	if r.Type == Null {
		return []Result{}
	}
	if !r.IsArray() {
		return []Result{r}
	}
	out := []Result{}
	r.ForEach(func(_, value Result) bool {
		out = append(out, value)
		return true
	})
	return out
}

// ForEach calls iterator for every member of an object (name and value) or
// element of an array (key of type Number, Num the index), in order, until
// it returns false or the bytes stop being valid JSON. A scalar is passed
// once, with an empty key.
func (r Result) ForEach(iterator func(key, value Result) bool) {
	if !r.Exists() {
		return
	}
	if r.Type != JSON {
		iterator(Result{}, r)
		return
	}
	d := getDecoder(r.data)
	defer putDecoder(d)
	dec := &d.dec
	object := r.data[0] == '{'
	if _, err := dec.ReadToken(); err != nil {
		return
	}
	for index := 0; ; index++ {
		if kind := dec.PeekKind(); kind == '}' || kind == ']' || kind == 0 {
			return
		}
		var key Result
		if object {
			var ok bool
			if key, ok = r.next(dec); !ok {
				return
			}
		} else {
			key = Result{Type: Number, Num: float64(index)}
		}
		value, ok := r.next(dec)
		if !ok || !iterator(key, value) {
			return
		}
	}
}

// next reads the next value of dec, which reads r.data; the value's text is
// a substring of r.Raw.
func (r Result) next(dec *jsontext.Decoder) (Result, bool) {
	value, err := dec.ReadValue()
	if err != nil {
		return Result{}, false
	}
	end := int(dec.InputOffset())
	start := end - len(value)
	return newResult(r.data[start:end], r.Raw[start:end]), true
}
