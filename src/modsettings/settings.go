// Package modsettings edits startup values without re-encoding unrelated property-tree bytes.
package modsettings

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"unicode/utf8"
)

const MaxSize = 16 << 20

var ErrConflict = errors.New("Settings changed since this page was loaded. Reload before saving.")

type node struct {
	kind       byte
	start, end int
	value      string
	children   map[string]*node
}
type parser struct {
	data       []byte
	pos, count int
}

func (p *parser) take(n int) ([]byte, error) {
	if n < 0 || n > len(p.data)-p.pos {
		return nil, errors.New("truncated settings file")
	}
	b := p.data[p.pos : p.pos+n]
	p.pos += n
	return b, nil
}
func (p *parser) str() (string, error) {
	b, e := p.take(1)
	if e != nil {
		return "", e
	}
	if b[0] == 1 {
		return "", nil
	}
	if b[0] != 0 {
		return "", errors.New("invalid string flag")
	}
	b, e = p.take(1)
	if e != nil {
		return "", e
	}
	n := uint32(b[0])
	if n == 255 {
		b, e = p.take(4)
		if e != nil {
			return "", e
		}
		n = binary.LittleEndian.Uint32(b)
	}
	if n > MaxSize {
		return "", errors.New("string too large")
	}
	b, e = p.take(int(n))
	if e != nil {
		return "", e
	}
	if !utf8.Valid(b) {
		return "", errors.New("invalid UTF-8")
	}
	return string(b), nil
}
func (p *parser) tree(depth int) (*node, error) {
	p.count++
	if depth > 64 || p.count > 100000 {
		return nil, errors.New("settings tree too complex")
	}
	start := p.pos
	b, e := p.take(2)
	if e != nil {
		return nil, e
	}
	n := &node{kind: b[0], start: start}
	if b[1] > 1 {
		return nil, errors.New("invalid property flag")
	}
	switch n.kind {
	case 0:
	case 1:
		b, e = p.take(1)
		if e == nil {
			if b[0] > 1 {
				return nil, errors.New("invalid boolean")
			}
			n.value = strconv.FormatBool(b[0] == 1)
		}
	case 2, 6, 7:
		b, e = p.take(8)
		if e == nil {
			v := binary.LittleEndian.Uint64(b)
			switch n.kind {
			case 2:
				f := math.Float64frombits(v)
				if math.IsNaN(f) || math.IsInf(f, 0) {
					return nil, errors.New("non-finite number")
				}
				n.value = strconv.FormatFloat(f, 'g', -1, 64)
			case 6:
				n.value = strconv.FormatInt(int64(v), 10)
			case 7:
				n.value = strconv.FormatUint(v, 10)
			}
		}
	case 3:
		n.value, e = p.str()
	case 4, 5:
		b, e = p.take(4)
		if e != nil {
			return nil, e
		}
		count := binary.LittleEndian.Uint32(b)
		if count > 100000 {
			return nil, errors.New("too many properties")
		}
		n.children = map[string]*node{}
		for i := uint32(0); i < count; i++ {
			key, err := p.str()
			if err != nil {
				return nil, err
			}
			child, err := p.tree(depth + 1)
			if err != nil {
				return nil, err
			}
			if n.kind == 5 {
				if _, ok := n.children[key]; ok {
					return nil, errors.New("duplicate property")
				}
				n.children[key] = child
			}
		}
	default:
		return nil, fmt.Errorf("unsupported property type %d", n.kind)
	}
	if e != nil {
		return nil, e
	}
	n.end = p.pos
	return n, nil
}

type Setting struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Value      string    `json:"value"`
	Components []Setting `json:"components,omitempty"`
}
type Document struct {
	Revision string    `json:"revision"`
	Settings []Setting `json:"settings"`
	raw      []byte
	values   map[string]*node
}

func revision(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func kindName(k byte) string {
	return map[byte]string{1: "bool", 2: "number", 3: "string", 6: "int", 7: "uint"}[k]
}
func Decode(raw []byte) (*Document, error) {
	if len(raw) < 9 || len(raw) > MaxSize || raw[8] != 0 {
		return nil, errors.New("invalid settings header or size")
	}
	p := parser{data: raw, pos: 9}
	root, e := p.tree(0)
	if e != nil {
		return nil, e
	}
	if p.pos != len(raw) || root.kind != 5 {
		return nil, errors.New("invalid settings root or trailing bytes")
	}
	startup := root.children["startup"]
	if startup == nil || startup.kind != 5 {
		return nil, errors.New("startup settings section missing")
	}
	d := &Document{Revision: revision(raw), raw: raw, values: map[string]*node{}, Settings: []Setting{}}
	for name, entry := range startup.children {
		if entry.kind != 5 || entry.children["value"] == nil {
			return nil, fmt.Errorf("invalid setting %s", name)
		}
		v := entry.children["value"]
		s := Setting{Name: name, Type: kindName(v.kind), Value: v.value}
		d.values[name] = v
		if v.kind == 5 {
			s.Type = "color"
			for _, key := range []string{"r", "g", "b", "a"} {
				c := v.children[key]
				if c != nil && (c.kind == 2 || c.kind == 6 || c.kind == 7) {
					s.Components = append(s.Components, Setting{Name: key, Type: kindName(c.kind), Value: c.value})
				} else if key != "a" {
					s.Type = "unsupported"
				}
			}
			if len(v.children) != len(s.Components) {
				s.Type = "unsupported"
			}
		}
		if s.Type == "" {
			s.Type = "unsupported"
		}
		d.Settings = append(d.Settings, s)
	}
	sort.Slice(d.Settings, func(i, j int) bool { return d.Settings[i].Name < d.Settings[j].Name })
	return d, nil
}
func Read(path string) (*Document, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() || info.Size() > MaxSize {
		return nil, errors.New("settings must be a regular file within the size limit")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	return Decode(raw)
}
func encode(n *node, value string) ([]byte, error) {
	b := new(bytes.Buffer)
	var e error
	switch n.kind {
	case 1:
		if value != "true" && value != "false" {
			return nil, errors.New("expected true or false")
		}
		if value == "true" {
			b.WriteByte(1)
		} else {
			b.WriteByte(0)
		}
	case 2:
		var v float64
		v, e = strconv.ParseFloat(value, 64)
		if e == nil && (math.IsNaN(v) || math.IsInf(v, 0)) {
			e = errors.New("expected a finite number")
		}
		if e == nil {
			e = binary.Write(b, binary.LittleEndian, v)
		}
	case 6:
		var v int64
		v, e = strconv.ParseInt(value, 10, 64)
		if e == nil {
			e = binary.Write(b, binary.LittleEndian, v)
		}
	case 7:
		var v uint64
		v, e = strconv.ParseUint(value, 10, 64)
		if e == nil {
			e = binary.Write(b, binary.LittleEndian, v)
		}
	case 3:
		if !utf8.ValidString(value) || len(value) > 1<<20 {
			return nil, errors.New("invalid or oversized text")
		}
		b.WriteByte(0)
		if len(value) < 255 {
			b.WriteByte(byte(len(value)))
		} else {
			b.WriteByte(255)
			binary.Write(b, binary.LittleEndian, uint32(len(value)))
		}
		b.WriteString(value)
	default:
		return nil, errors.New("unsupported setting type")
	}
	return b.Bytes(), e
}

// Values are strings so 64-bit integers survive browser JSON without precision loss.
type Change struct {
	Value      *string           `json:"value,omitempty"`
	Components map[string]string `json:"components,omitempty"`
}
type patch struct {
	start, end int
	bytes      []byte
}

func (d *Document) Apply(changes map[string]Change) ([]byte, error) {
	patches := []patch{}
	add := func(n *node, v string) error {
		if v == n.value {
			return nil
		}
		b, e := encode(n, v)
		if e != nil {
			return e
		}
		patches = append(patches, patch{n.start + 2, n.end, b})
		return nil
	}
	for name, change := range changes {
		n, ok := d.values[name]
		if !ok {
			return nil, fmt.Errorf("unknown startup setting %s", name)
		}
		if n.kind == 5 {
			if change.Value != nil || len(change.Components) == 0 {
				return nil, fmt.Errorf("%s: expected color components", name)
			}
			supported := false
			for _, s := range d.Settings {
				if s.Name == name && s.Type == "color" {
					supported = true
				}
			}
			if !supported {
				return nil, errors.New("unsupported setting")
			}
			for key, value := range change.Components {
				c := n.children[key]
				if c == nil {
					return nil, errors.New("unknown color component")
				}
				f, e := strconv.ParseFloat(value, 64)
				if e != nil || math.IsNaN(f) || f < 0 || f > 1 {
					return nil, errors.New("color components must be between 0 and 1")
				}
				if e = add(c, value); e != nil {
					return nil, e
				}
			}
		} else {
			if change.Value == nil || len(change.Components) > 0 {
				return nil, fmt.Errorf("%s: expected a value", name)
			}
			if e := add(n, *change.Value); e != nil {
				return nil, fmt.Errorf("%s: %w", name, e)
			}
		}
	}
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	out := new(bytes.Buffer)
	pos := 0
	for _, p := range patches {
		out.Write(d.raw[pos:p.start])
		out.Write(p.bytes)
		pos = p.end
	}
	out.Write(d.raw[pos:])
	if out.Len() > MaxSize {
		return nil, errors.New("settings too large")
	}
	return out.Bytes(), nil
}
func Save(path, expected string, changes map[string]Change) (*Document, error) {
	d, e := Read(path)
	if e != nil {
		return nil, e
	}
	if expected != d.Revision {
		return nil, ErrConflict
	}
	raw, e := d.Apply(changes)
	if e != nil {
		return nil, e
	}
	if bytes.Equal(raw, d.raw) {
		return d, nil
	}
	result, e := Decode(raw)
	if e != nil {
		return nil, e
	}
	dir := filepath.Join(filepath.Dir(path), ".startup-settings-backups")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	backup := filepath.Join(dir, d.Revision+".dat")
	f, e := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		old, readErr := os.ReadFile(backup)
		if readErr != nil || !bytes.Equal(old, d.raw) {
			return nil, errors.New("existing backup does not match")
		}
	} else if e != nil {
		return nil, e
	} else {
		_, e = f.Write(d.raw)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil {
			return nil, e
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	temp, e := os.CreateTemp(filepath.Dir(path), ".startup-settings-*")
	if e != nil {
		return nil, e
	}
	defer os.Remove(temp.Name())
	_, e = temp.Write(raw)
	if e == nil {
		e = temp.Sync()
	}
	closeErr := temp.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	// Recheck after preparing the replacement so external uploads cannot be silently overwritten.
	current, e := Read(path)
	if e != nil {
		return nil, e
	}
	if current.Revision != expected {
		return nil, ErrConflict
	}
	if e = os.Rename(temp.Name(), path); e != nil {
		return nil, e
	}
	return result, nil
}
