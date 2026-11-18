package vault

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fabianramirez-code/qrypt/internal/identity"
)

func FuzzCheckSecretFile(f *testing.F) {
	valid := make([]byte, valueOffset+minValueSealed)
	copy(valid, secretMagic)
	valid[magicSize] = formatVersion
	binary.BigEndian.PutUint32(valid[valueSizeOffset:], minValueSealed)
	item := make([]byte, bodyOffset+minValueSealed)
	copy(item, secretMagic)
	item[magicSize] = itemVersion
	binary.BigEndian.PutUint32(item[bodyNonceEnd:], minValueSealed)
	for _, seed := range [][]byte{nil, []byte(secretMagic), valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0), item, item[:len(item)-1], append(bytes.Clone(item), 0)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, file []byte) {
		if err := checkSecretFile(file); err == nil {
			version := file[magicSize]
			if string(file[:magicSize]) != secretMagic || (version != formatVersion && version != itemVersion) {
				t.Fatal("accepted wrong magic or version")
			}
			offset, sizeOffset := valueOffset, valueSizeOffset
			if version == itemVersion {
				offset, sizeOffset = bodyOffset, bodyNonceEnd
			}
			size := binary.BigEndian.Uint32(file[sizeOffset:offset])
			if size != uint32(len(file)-offset) || len(file) < offset+minValueSealed || len(file) > offset+maxValueSealed || size < minValueSealed || (size-tagSize)%valueBlock != 0 {
				t.Fatal("accepted invalid sealed body layout")
			}
		}
	})
}

func FuzzDecodeFields(f *testing.F) {
	valid := encodeBody(Item{Fields: []Field{{Kind: FieldHidden, Label: "value", Value: []byte("sample")}}})
	for _, seed := range [][]byte{nil, {0}, {1}, {0, 1}, {1, byte(FieldHidden), 1, 'x', 0, 0, 0, 1, 'a'}, valid[4 : 4+int(binary.BigEndian.Uint32(valid))]} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		plain := make([]byte, 4+len(data))
		binary.BigEndian.PutUint32(plain, uint32(len(data)))
		copy(plain[4:], data)
		fields, err := decodeFields(plain)
		if err != nil {
			return
		}
		encoded := encodeBody(Item{Fields: fields})
		again, err := decodeFields(encoded)
		if err != nil || !reflect.DeepEqual(fields, again) {
			t.Fatalf("fields round trip: %v, %v, %v", fields, again, err)
		}
	})
}

func FuzzVerifyManifest(f *testing.F) {
	id, err := identity.FromSeed(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		f.Fatal(err)
	}
	public := id.PublicKeys()
	v := newVault("", make([]byte, scopeIDSize), make([]byte, keySize), id)
	signed, err := v.signManifest(Manifest{Generation: 1, Files: map[string][32]byte{}})
	v.Close()
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{nil, []byte(manifestMagic), signed, signed[:len(signed)-1], append(bytes.Clone(signed), 0), bytes.Clone(signed)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, err := verifyManifest(public.Ed25519, public.MLDSA65, data)
		if bytes.Equal(data, signed) {
			if err != nil {
				t.Fatalf("signed manifest rejected: %v", err)
			}
		} else if !errors.Is(err, ErrIntegrity) {
			t.Fatalf("unsigned data accepted or wrong error: %v", err)
		}
	})
}

func FuzzReadRecord(f *testing.F) {
	valid := []byte("00000000000000000000000000000000 1 " + string(bytes.Repeat([]byte{'a'}, 64)) + "\n")
	for _, seed := range [][]byte{nil, valid, valid[:len(valid)-1], append(bytes.Clone(valid), 0), []byte("\n"), []byte("bad record\n")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "record")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		entries, err := readRecord(path)
		if err != nil {
			return
		}
		if err := writeRecord(path, entries); err != nil {
			t.Fatal(err)
		}
		again, err := readRecord(path)
		if err != nil || !reflect.DeepEqual(entries, again) {
			t.Fatalf("record round trip: %v, %v, %v", entries, again, err)
		}
	})
}
