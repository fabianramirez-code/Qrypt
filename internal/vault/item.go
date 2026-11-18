package vault

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

const (
	itemVersion      = 2
	headSize         = 768
	pathSize         = 512
	headEnd          = nameSegmentOffset + headSize + tagSize
	bodyNonceEnd     = headEnd + nonceSize
	bodyOffset       = bodyNonceEnd + 4
	maxPathEncoded   = 511
	maxFieldsEncoded = 65788
	maxItemFile      = bodyOffset + maxValueSealed
)

type FieldKind byte

const (
	FieldHidden  FieldKind = 1
	FieldVisible FieldKind = 2
	FieldNote    FieldKind = 3
)

type Field struct {
	Kind  FieldKind
	Label string
	Value []byte
}
type Entry struct {
	ID    string
	Path  []string
	Title string
}
type Item struct {
	Path   []string
	Title  string
	Fields []Field
}

var (
	ErrLabelExists   = errors.New("a field with that label already exists")
	ErrTooManyFields = errors.New("too many fields")
	ErrPathTooLong   = errors.New("folder path too long")
	ErrInvalidField  = errors.New("invalid field")
)

func validatePath(path []string) error {
	if len(path) > 255 {
		return ErrPathTooLong
	}
	length := 1
	for _, part := range path {
		if err := validateName(part); err != nil {
			return err
		}
		length += 1 + len(part)
		if length > maxPathEncoded {
			return ErrPathTooLong
		}
	}
	return nil
}

func validateItem(item Item) error {
	if err := validateName(item.Title); err != nil {
		return err
	}
	if err := validatePath(item.Path); err != nil {
		return err
	}
	if len(item.Fields) > 64 {
		return ErrTooManyFields
	}
	length := 1
	labels := make(map[string]bool, len(item.Fields))
	for _, field := range item.Fields {
		if err := validateName(field.Label); err != nil {
			return err
		}
		if labels[field.Label] {
			return ErrLabelExists
		}
		labels[field.Label] = true
		if field.Kind < FieldHidden || field.Kind > FieldNote || (field.Kind != FieldNote && bytes.ContainsAny(field.Value, "\r\n")) {
			return ErrInvalidField
		}
		length += 6 + len(field.Label) + len(field.Value)
		if length > maxFieldsEncoded {
			return ErrValueTooLarge
		}
	}
	return nil
}

func (v *Vault) itemAAD(fid []byte, version byte, segment byte) []byte {
	aad := v.aad(fid, segment)
	aad[magicSize] = version
	return aad
}

func (v *Vault) itemHead(fid, file []byte) (Entry, error) {
	if err := checkSecretFile(file); err != nil {
		return Entry{}, err
	}
	version := file[magicSize]
	end := valueNonceOffset
	if version == itemVersion {
		end = headEnd
	}
	plain, err := open(v.nameKey, file[nameNonceOffset:nameSegmentOffset], file[nameSegmentOffset:end], v.itemAAD(fid, version, segmentName))
	if err != nil {
		return Entry{}, err
	}
	defer clear(plain)
	n := int(plain[0])
	if n == 0 || validateName(string(plain[1:1+n])) != nil || !allZero(plain[1+n:namePadded]) {
		return Entry{}, ErrInvalidFormat
	}
	entry := Entry{ID: hex.EncodeToString(fid), Title: string(plain[1 : 1+n])}
	if version == formatVersion {
		return entry, nil
	}
	path := plain[namePadded:]
	count := int(path[0])
	off := 1
	if count > 0 {
		entry.Path = make([]string, 0, count)
	}
	for i := 0; i < count; i++ {
		if off >= len(path) {
			return Entry{}, ErrInvalidFormat
		}
		size := int(path[off])
		off++
		if size == 0 || off+size > len(path) {
			return Entry{}, ErrInvalidFormat
		}
		name := string(path[off : off+size])
		off += size
		if validateName(name) != nil {
			return Entry{}, ErrInvalidFormat
		}
		entry.Path = append(entry.Path, name)
	}
	if off > maxPathEncoded || !allZero(path[off:]) {
		return Entry{}, ErrInvalidFormat
	}
	return entry, nil
}

// itemBody opens only the body, returning its padded plaintext; callers clear it.
var itemBodyOpened func()

func (v *Vault) itemBody(fid, file []byte) ([]byte, error) {
	if itemBodyOpened != nil {
		itemBodyOpened()
	}
	if err := checkSecretFile(file); err != nil {
		return nil, err
	}
	nonce, size := valueNonceOffset, valueOffset
	if file[magicSize] == itemVersion {
		nonce, size = headEnd, bodyOffset
	}
	plain, err := open(v.valueKey, file[nonce:nonce+nonceSize], file[size:], v.itemAAD(fid, file[magicSize], segmentValue))
	if err != nil {
		return nil, err
	}
	if len(plain) < 4 {
		clear(plain)
		return nil, ErrInvalidFormat
	}
	length := int(binary.BigEndian.Uint32(plain))
	limit := MaxValueSize
	if file[magicSize] == itemVersion {
		limit = maxFieldsEncoded
	}
	if length > limit || length > len(plain)-4 || len(plain) != (4+length+255)/256*256 || !allZero(plain[4+length:]) {
		clear(plain)
		return nil, ErrInvalidFormat
	}
	return plain, nil
}

func decodeFields(plain []byte) ([]Field, error) {
	data := plain[4 : 4+int(binary.BigEndian.Uint32(plain))]
	if len(data) < 1 || data[0] > 64 {
		return nil, ErrInvalidFormat
	}
	var fields []Field
	off := 1
	for i := 0; i < int(data[0]); i++ {
		if off+2 > len(data) {
			return nil, ErrInvalidFormat
		}
		kind := FieldKind(data[off])
		labelSize := int(data[off+1])
		off += 2
		if labelSize == 0 || off+labelSize+4 > len(data) {
			return nil, ErrInvalidFormat
		}
		label := string(data[off : off+labelSize])
		off += labelSize
		n := uint64(binary.BigEndian.Uint32(data[off:]))
		off += 4
		if n > uint64(len(data)-off) {
			return nil, ErrInvalidFormat
		}
		var value []byte
		if n > 0 {
			value = bytes.Clone(data[off : off+int(n)])
		}
		off += int(n)
		fields = append(fields, Field{Kind: kind, Label: label, Value: value})
	}
	if off != len(data) || validateItem(Item{Title: "valid", Fields: fields}) != nil {
		for _, field := range fields {
			clear(field.Value)
		}
		return nil, ErrInvalidFormat
	}
	return fields, nil
}

func (v *Vault) itemRead(fid, file []byte) (Item, error) {
	entry, err := v.itemHead(fid, file)
	if err != nil {
		return Item{}, err
	}
	plain, err := v.itemBody(fid, file)
	if err != nil {
		return Item{}, err
	}
	defer clear(plain)
	item := Item{Title: entry.Title, Path: entry.Path}
	if file[magicSize] == formatVersion {
		value := bytes.Clone(plain[4 : 4+int(binary.BigEndian.Uint32(plain))])
		kind := FieldHidden
		if bytes.ContainsAny(value, "\r\n") {
			kind = FieldNote
		}
		item.Fields = []Field{{Kind: kind, Label: "value", Value: value}}
	} else {
		item.Fields, err = decodeFields(plain)
	}
	return item, err
}

func encodeBody(item Item) []byte {
	data := []byte{byte(len(item.Fields))}
	for _, f := range item.Fields {
		data = append(data, byte(f.Kind), byte(len(f.Label)))
		data = append(data, f.Label...)
		data = binary.BigEndian.AppendUint32(data, uint32(len(f.Value)))
		data = append(data, f.Value...)
	}
	padded := make([]byte, (4+len(data)+255)/256*256)
	binary.BigEndian.PutUint32(padded, uint32(len(data)))
	copy(padded[4:], data)
	clear(data)
	return padded
}

func (v *Vault) encodeItem(fid []byte, item Item, body []byte) ([]byte, error) {
	head := make([]byte, headSize)
	defer clear(head)
	head[0] = byte(len(item.Title))
	copy(head[1:], item.Title)
	path := head[namePadded:]
	path[0] = byte(len(item.Path))
	off := 1
	for _, part := range item.Path {
		path[off] = byte(len(part))
		off++
		copy(path[off:], part)
		off += len(part)
	}
	headNonce := make([]byte, nonceSize)
	bodyNonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, headNonce); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand.Reader, bodyNonce); err != nil {
		return nil, err
	}
	sealedHead, err := seal(v.nameKey, headNonce, head, v.itemAAD(fid, itemVersion, segmentName))
	if err != nil {
		return nil, err
	}
	sealedBody, err := seal(v.valueKey, bodyNonce, body, v.itemAAD(fid, itemVersion, segmentValue))
	if err != nil {
		return nil, err
	}
	file := make([]byte, 0, bodyOffset+len(sealedBody))
	file = append(file, secretMagic...)
	file = append(file, itemVersion)
	file = append(file, headNonce...)
	file = append(file, sealedHead...)
	file = append(file, bodyNonce...)
	file = binary.BigEndian.AppendUint32(file, uint32(len(sealedBody)))
	return append(file, sealedBody...), nil
}

func (v *Vault) itemScan() ([]Entry, error) {
	if err := v.ready(); err != nil {
		return nil, err
	}
	files, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var result []Entry
	for _, f := range files {
		fid, ok := parseFileName(f.Name())
		if !ok || f.IsDir() {
			continue
		}
		blob, e := readLimited(v.secretPath(fid), maxItemFile)
		if e != nil {
			return nil, e
		}
		if e = v.verifyFile(fid, blob); e != nil {
			return nil, e
		}
		head, e := v.itemHead(fid, blob)
		if e != nil {
			return nil, e
		}
		result = append(result, head)
	}
	return result, nil
}

func (v *Vault) Entries() ([]Entry, error) {
	result, err := v.itemScan()
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		for k := 0; k < len(a.Path) && k < len(b.Path); k++ {
			if a.Path[k] != b.Path[k] {
				return a.Path[k] < b.Path[k]
			}
		}
		if len(a.Path) != len(b.Path) {
			return len(a.Path) < len(b.Path)
		}
		return a.Title < b.Title
	})
	return result, nil
}

func decodeID(id string) ([]byte, error) {
	if len(id) != 32 {
		return nil, ErrNotFound
	}
	fid, err := hex.DecodeString(id)
	if err != nil || hex.EncodeToString(fid) != id {
		return nil, ErrNotFound
	}
	return fid, nil
}

func (v *Vault) fileByID(id string) ([]byte, []byte, error) {
	fid, err := decodeID(id)
	if err != nil {
		return nil, nil, err
	}
	if err = v.ready(); err != nil {
		return nil, nil, err
	}
	if _, ok := v.manifest.Files[id]; !ok {
		return nil, nil, ErrNotFound
	}
	blob, err := readLimited(v.secretPath(fid), maxItemFile)
	if err != nil {
		return nil, nil, err
	}
	if err = v.verifyFile(fid, blob); err != nil {
		return nil, nil, err
	}
	return fid, blob, nil
}

func (v *Vault) Read(id string) (Item, error) {
	fid, blob, err := v.fileByID(id)
	if err != nil {
		return Item{}, err
	}
	return v.itemRead(fid, blob)
}

func (v *Vault) checkCollision(item Item, except string) error {
	entries, err := v.itemScan()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.ID != except && e.Title == item.Title && slices.Equal(e.Path, item.Path) {
			return ErrNameExists
		}
	}
	return nil
}

func (v *Vault) Add(item Item) (string, error) {
	if err := validateItem(item); err != nil {
		return "", err
	}
	if err := v.checkCollision(item, ""); err != nil {
		return "", err
	}
	fid := make([]byte, fileIDSize)
	if _, err := io.ReadFull(rand.Reader, fid); err != nil {
		return "", err
	}
	id := hex.EncodeToString(fid)
	body := encodeBody(item)
	defer clear(body)
	blob, err := v.encodeItem(fid, item, body)
	if err != nil {
		return "", err
	}
	m := v.nextManifest()
	m.Files[id] = sha256.Sum256(blob)
	if err = v.change(m, func() error { return writeNew(v.secretPath(fid), blob) }); err != nil {
		return "", err
	}
	return id, nil
}

func (v *Vault) Replace(id string, item Item) error {
	if err := validateItem(item); err != nil {
		return err
	}
	fid, _, err := v.fileByID(id)
	if err != nil {
		return err
	}
	if err = v.checkCollision(item, id); err != nil {
		return err
	}
	body := encodeBody(item)
	defer clear(body)
	blob, err := v.encodeItem(fid, item, body)
	if err != nil {
		return err
	}
	m := v.nextManifest()
	m.Files[id] = sha256.Sum256(blob)
	return v.change(m, func() error { return writeReplacing(v.secretPath(fid), blob) })
}
func (v *Vault) Remove(id string) error {
	fid, _, err := v.fileByID(id)
	if err != nil {
		return err
	}
	m := v.nextManifest()
	delete(m.Files, id)
	return v.change(m, func() error { return os.Remove(v.secretPath(fid)) })
}

func hasPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && slices.Equal(path[:len(prefix)], prefix)
}

// moveFolderStep is an internal failure seam used by interrupted-move tests.
var moveFolderStep func(int) error

func (v *Vault) MoveFolder(from, to []string) error {
	if len(from) == 0 {
		return ErrInvalidName
	}
	if err := validatePath(from); err != nil {
		return err
	}
	if err := validatePath(to); err != nil {
		return err
	}
	if hasPrefix(to, from) {
		return ErrInvalidField
	}
	entries, err := v.itemScan()
	if err != nil {
		return err
	}
	moving := make([]Entry, 0)
	occupied := make(map[string]bool, len(entries))
	for _, e := range entries {
		path := e.Path
		if hasPrefix(path, from) {
			path = append(append([]string{}, to...), path[len(from):]...)
			if err := validatePath(path); err != nil {
				return err
			}
			moving = append(moving, e)
		} else {
			occupied[pathKey(path, e.Title)] = true
		}
	}
	for _, e := range moving {
		path := append(append([]string{}, to...), e.Path[len(from):]...)
		key := pathKey(path, e.Title)
		if occupied[key] {
			return ErrNameExists
		}
		occupied[key] = true
	}
	for i, e := range moving {
		fid, blob, err := v.fileByID(e.ID)
		if err != nil {
			return err
		}
		newPath := append(append([]string{}, to...), e.Path[len(from):]...)
		var body []byte
		if blob[magicSize] == formatVersion {
			item, readErr := v.itemRead(fid, blob)
			if readErr != nil {
				return readErr
			}
			item.Path = newPath
			body = encodeBody(item)
			for _, f := range item.Fields {
				clear(f.Value)
			}
		} else {
			body, err = v.itemBody(fid, blob)
			if err != nil {
				return err
			}
		}
		newBlob, err := v.encodeItem(fid, Item{Title: e.Title, Path: newPath}, body)
		clear(body)
		if err != nil {
			return err
		}
		m := v.nextManifest()
		m.Files[e.ID] = sha256.Sum256(newBlob)
		if err = v.change(m, func() error { return writeReplacing(v.secretPath(fid), newBlob) }); err != nil {
			return err
		}
		if moveFolderStep != nil {
			if err = moveFolderStep(i + 1); err != nil {
				return err
			}
		}
	}
	return nil
}
func pathKey(path []string, title string) string {
	var b []byte
	for _, part := range path {
		b = binary.BigEndian.AppendUint32(b, uint32(len(part)))
		b = append(b, part...)
	}
	b = binary.BigEndian.AppendUint32(b, uint32(len(title)))
	b = append(b, title...)
	return string(b)
}
