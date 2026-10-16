package modsettings

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func textBytes(s string) []byte {
	b := new(bytes.Buffer)
	b.WriteByte(0)
	if len(s) < 255 {
		b.WriteByte(byte(len(s)))
	} else {
		b.WriteByte(255)
		binary.Write(b, binary.LittleEndian, uint32(len(s)))
	}
	b.WriteString(s)
	return b.Bytes()
}
func scalar(kind byte, v interface{}) []byte {
	b := new(bytes.Buffer)
	b.Write([]byte{kind, 0})
	if kind == 3 {
		b.Write(textBytes(v.(string)))
	} else {
		binary.Write(b, binary.LittleEndian, v)
	}
	return b.Bytes()
}

type pair struct {
	k string
	v []byte
}

func dict(pairs ...pair) []byte {
	b := new(bytes.Buffer)
	b.Write([]byte{5, 0})
	binary.Write(b, binary.LittleEndian, uint32(len(pairs)))
	for _, p := range pairs {
		b.Write(textBytes(p.k))
		b.Write(p.v)
	}
	return b.Bytes()
}
func fixture() []byte {
	startup := dict(pair{"toggle", dict(pair{"value", scalar(1, byte(1))})}, pair{"number", dict(pair{"value", scalar(2, 2.5)})}, pair{"text", dict(pair{"value", scalar(3, "old")})}, pair{"signed", dict(pair{"value", scalar(6, int64(math.MinInt64))})}, pair{"unsigned", dict(pair{"value", scalar(7, uint64(math.MaxUint64))})}, pair{"tint", dict(pair{"value", dict(pair{"r", scalar(2, 0.5)}, pair{"g", scalar(2, 0.5)}, pair{"b", scalar(2, 0.5)}, pair{"a", scalar(2, 1.0)})})})
	root := dict(pair{"startup", startup}, pair{"runtime-global", dict(pair{"keep", scalar(3, "runtime settings must not change")})}, pair{"runtime-per-user", dict()})
	return append([]byte{2, 0, 1, 0, 22, 0, 0, 0, 0}, root...)
}
func value(v string) Change { return Change{Value: &v} }
func TestRoundTripAndExactIntegers(t *testing.T) {
	raw := fixture()
	d, e := Decode(raw)
	if e != nil {
		t.Fatal(e)
	}
	out, e := d.Apply(map[string]Change{})
	if e != nil || !bytes.Equal(out, raw) {
		t.Fatal("no-op changed bytes", e)
	}
	if d.values["unsigned"].value != "18446744073709551615" || d.values["signed"].value != "-9223372036854775808" {
		t.Fatal("integer precision lost")
	}
	changes := map[string]Change{"toggle": value("false"), "text": value(strings.Repeat("é", 300)), "number": value("8.25"), "signed": value("-9223372036854775807"), "tint": {Components: map[string]string{"r": "0.75"}}}
	out, e = d.Apply(changes)
	if e != nil {
		t.Fatal(e)
	}
	after, e := Decode(out)
	if e != nil {
		t.Fatal(e)
	}
	if after.values["text"].value != strings.Repeat("é", 300) || after.values["toggle"].value != "false" || after.values["number"].value != "8.25" {
		t.Fatal("values incorrect")
	}
	if !bytes.Equal(out[:9], raw[:9]) {
		t.Fatal("header changed")
	}
	tail := raw[bytes.Index(raw, textBytes("runtime-global")):]
	if !bytes.HasSuffix(out, tail) {
		t.Fatal("runtime bytes changed")
	}
}
func TestRejectInvalidChanges(t *testing.T) {
	d, _ := Decode(fixture())
	for _, changes := range []map[string]Change{
		{"missing": value("1")}, {"toggle": value("1")}, {"number": value("NaN")}, {"number": value("Inf")}, {"signed": value("1.5")}, {"unsigned": value("18446744073709551616")}, {"unsigned": value("-1")}, {"text": {}}, {"tint": value("oops")}, {"tint": {Components: map[string]string{"r": "1.1"}}}, {"tint": {Components: map[string]string{"other": "0"}}},
	} {
		if _, e := d.Apply(changes); e == nil {
			t.Fatalf("accepted invalid change %v", changes)
		}
	}
}
func TestMalformedInput(t *testing.T) {
	raw := fixture()
	for i := 0; i < len(raw); i++ {
		if _, e := Decode(raw[:i]); e == nil {
			t.Fatalf("accepted truncation %d", i)
		}
	}
	if _, e := Decode(append(raw, 0)); e == nil {
		t.Fatal("accepted trailing data")
	}
	bad := append([]byte{}, raw...)
	bad[9] = 99
	if _, e := Decode(bad); e == nil {
		t.Fatal("accepted unknown type")
	}
}
func TestAtomicSaveBackupAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mod-settings.dat")
	raw := fixture()
	os.WriteFile(path, raw, 0600)
	d, _ := Read(path)
	result, e := Save(path, d.Revision, map[string]Change{"number": value("100")})
	if e != nil {
		t.Fatal(e)
	}
	backup, e := os.ReadFile(filepath.Join(filepath.Dir(path), ".startup-settings-backups", d.Revision+".dat"))
	if e != nil || !bytes.Equal(raw, backup) {
		t.Fatal("backup mismatch", e)
	}
	if _, e = Save(path, d.Revision, map[string]Change{"number": value("200")}); !errors.Is(e, ErrConflict) {
		t.Fatal("stale write accepted", e)
	}
	if _, e = Save(path, result.Revision, map[string]Change{"number": value("NaN")}); e == nil {
		t.Fatal("invalid write accepted")
	}
	unchanged, _ := os.ReadFile(path)
	if revision(unchanged) != result.Revision {
		t.Fatal("failed request changed file")
	}
}
func FuzzDecode(f *testing.F) {
	f.Add(fixture())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, raw []byte) {
		d, e := Decode(raw)
		if e == nil {
			out, e := d.Apply(nil)
			if e != nil || !bytes.Equal(out, raw) {
				t.Fatal("no-op did not preserve bytes")
			}
		}
	})
}
