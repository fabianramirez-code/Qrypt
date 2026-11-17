package vault

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
	"github.com/fabianramirez-code/qrypt/internal/identity"
)

const manifestMagic = "QRYPTSM"
const manifestLabel = "qrypt/v1/vault/manifest"
const manifestHeader = 68
const manifestEntry = 48
const manifestSignatures = ed25519.SignatureSize + mldsa65.SignatureSize

var ErrIntegrity = errors.New("vault integrity problem")

type ProblemKind string

const (
	ProblemFiles     ProblemKind = "files"
	ProblemSignature ProblemKind = "signature"
	ProblemFork      ProblemKind = "fork"
	ProblemRollback  ProblemKind = "rollback"
	ProblemRemoved   ProblemKind = "removed"
)

type Problem struct {
	ScopeID                                     string
	Name                                        string
	Kind                                        ProblemKind
	Missing, Unexpected, Changed, Undecryptable int
	Generation, Recorded                        uint64
}

func (p *Problem) Error() string {
	return "vault integrity problem: " + string(p.Kind) + " in " + p.ScopeID
}
func (p *Problem) Is(err error) bool { return err == ErrIntegrity }

type Manifest struct {
	ScopeID    string
	Generation uint64
	NameHash   [32]byte
	Files      map[string][32]byte
	Hash       [32]byte
}

func VerifyManifest(id *identity.Identity, data []byte) (Manifest, error) {
	public := id.PublicKeys()
	return verifyManifest(public.Ed25519, public.MLDSA65, data)
}
func verifyManifest(ed, ml []byte, data []byte) (Manifest, error) {
	var m Manifest
	if len(data) < manifestHeader+manifestSignatures || (len(data)-manifestHeader-manifestSignatures)%manifestEntry != 0 || string(data[:7]) != manifestMagic || data[7] != formatVersion {
		return m, ErrIntegrity
	}
	count := int(binary.BigEndian.Uint32(data[64:68]))
	if count != (len(data)-manifestHeader-manifestSignatures)/manifestEntry || binary.BigEndian.Uint64(data[24:32]) == 0 {
		return m, ErrIntegrity
	}
	var pub mldsa65.PublicKey
	if err := pub.UnmarshalBinary(ml); err != nil {
		return m, ErrIntegrity
	}
	unsigned := data[:len(data)-manifestSignatures]
	msg := append([]byte(manifestLabel), unsigned...)
	if !ed25519.Verify(ed, msg, data[len(unsigned):len(unsigned)+ed25519.SignatureSize]) || !mldsa65.Verify(&pub, msg, nil, data[len(unsigned)+ed25519.SignatureSize:]) {
		return m, ErrIntegrity
	}
	m.ScopeID = hex.EncodeToString(data[8:24])
	m.Generation = binary.BigEndian.Uint64(data[24:32])
	copy(m.NameHash[:], data[32:64])
	m.Files = make(map[string][32]byte, count)
	var previous string
	for i := 0; i < count; i++ {
		off := manifestHeader + i*manifestEntry
		key := hex.EncodeToString(data[off : off+16])
		if i > 0 && key <= previous {
			return Manifest{}, ErrIntegrity
		}
		previous = key
		var h [32]byte
		copy(h[:], data[off+16:off+48])
		m.Files[key] = h
	}
	m.Hash = sha256.Sum256(data)
	return m, nil
}

func (v *Vault) Generation() uint64     { return v.manifest.Generation }
func (v *Vault) ManifestHash() [32]byte { return v.manifest.Hash }
func (v *Vault) problem(kind ProblemKind) *Problem {
	p := &Problem{ScopeID: v.ID(), Kind: kind, Generation: v.manifest.Generation}
	if v.scopeNameKey != nil {
		p.Name, _ = v.nameUnchecked()
	}
	return p
}

func (v *Vault) signManifest(m Manifest) ([]byte, error) {
	keys := make([]string, 0, len(m.Files))
	for k := range m.Files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	data := make([]byte, manifestHeader+len(keys)*manifestEntry+manifestSignatures)
	copy(data, manifestMagic)
	data[7] = formatVersion
	copy(data[8:24], v.scopeID)
	binary.BigEndian.PutUint64(data[24:32], m.Generation)
	copy(data[32:64], m.NameHash[:])
	binary.BigEndian.PutUint32(data[64:68], uint32(len(keys)))
	for i, k := range keys {
		fid, _ := hex.DecodeString(k)
		copy(data[manifestHeader+i*manifestEntry:], fid)
		hash := m.Files[k]
		copy(data[manifestHeader+i*manifestEntry+16:], hash[:])
	}
	unsigned := data[:len(data)-manifestSignatures]
	msg := append([]byte(manifestLabel), unsigned...)
	copy(data[len(unsigned):], ed25519.Sign(v.edSigning, msg))
	var seed [mldsa65.SeedSize]byte
	copy(seed[:], v.mlSeed)
	defer clear(seed[:])
	_, private := mldsa65.NewKeyFromSeed(&seed)
	err := mldsa65.SignTo(private, msg, nil, false, data[len(unsigned)+ed25519.SignatureSize:])
	*private = mldsa65.PrivateKey{}
	return data, err
}
func (v *Vault) nextManifest() Manifest {
	m := Manifest{ScopeID: v.ID(), Generation: v.manifest.Generation + 1, NameHash: v.manifest.NameHash, Files: make(map[string][32]byte, len(v.manifest.Files))}
	for key, h := range v.manifest.Files {
		m.Files[key] = h
	}
	return m
}
func (v *Vault) change(m Manifest, action func() error) error {
	data, err := v.signManifest(m)
	if err != nil {
		return err
	}
	next := filepath.Join(v.dir, "manifest.next")
	if err = writeReplacing(next, data); err != nil {
		return err
	}
	if err = action(); err != nil {
		// The change did not happen, so its announcement must not stay behind.
		os.Remove(next)
		return err
	}
	if err = os.Rename(next, filepath.Join(v.dir, "manifest")); err != nil {
		return err
	}
	m.Hash = sha256.Sum256(data)
	v.manifest = m
	return nil
}

// diskFiles counts changes without interpreting ciphertext. Only 32-hex .qrs files are secrets.
func (v *Vault) diskFiles(m Manifest) (*Problem, error) {
	p := v.problem(ProblemFiles)
	name, err := os.ReadFile(filepath.Join(v.dir, scopeNameFile))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var nameHash [32]byte
	if err == nil {
		nameHash = sha256.Sum256(name)
	}
	if nameHash != m.NameHash {
		if err != nil {
			p.Missing++
		} else if m.NameHash == ([32]byte{}) {
			p.Unexpected++
		} else {
			p.Changed++
		}
	}
	entries, err := os.ReadDir(filepath.Join(v.dir, secretDir))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id, ok := parseFileName(e.Name())
		if !ok {
			continue
		}
		key := hex.EncodeToString(id)
		seen[key] = true
		expected, found := m.Files[key]
		if !found {
			p.Unexpected++
			continue
		}
		data, err := os.ReadFile(filepath.Join(v.dir, secretDir, e.Name()))
		if err != nil {
			return nil, err
		}
		if sha256.Sum256(data) != expected {
			p.Changed++
		}
	}
	for key := range m.Files {
		if !seen[key] {
			p.Missing++
		}
	}
	if p.Missing+p.Unexpected+p.Changed != 0 {
		return p, nil
	}
	return nil, nil
}
func (v *Vault) verifyFile(id []byte, data []byte) error {
	expected, ok := v.manifest.Files[hex.EncodeToString(id)]
	if !ok || sha256.Sum256(data) != expected {
		p := v.problem(ProblemFiles)
		if ok {
			p.Changed = 1
		} else {
			p.Unexpected = 1
		}
		return p
	}
	return nil
}
func (v *Vault) checkDisk() error {
	data, err := os.ReadFile(filepath.Join(v.dir, "manifest"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return v.problem(ProblemSignature)
		}
		return err
	}
	if sha256.Sum256(data) != v.manifest.Hash {
		return v.problem(ProblemSignature)
	}
	p, err := v.diskFiles(v.manifest)
	if err != nil {
		return err
	}
	if p != nil {
		return p
	}
	return nil
}
func (v *Vault) openManifest() error {
	path := filepath.Join(v.dir, "manifest")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Old scopes gain their first manifest from the files already on disk.
		m := Manifest{ScopeID: v.ID(), Generation: 1, Files: make(map[string][32]byte)}
		name, e := os.ReadFile(filepath.Join(v.dir, scopeNameFile))
		if e == nil {
			m.NameHash = sha256.Sum256(name)
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		entries, e := os.ReadDir(filepath.Join(v.dir, secretDir))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		for _, item := range entries {
			fid, ok := parseFileName(item.Name())
			if !ok || item.IsDir() {
				continue
			}
			blob, e := os.ReadFile(filepath.Join(v.dir, secretDir, item.Name()))
			if e != nil {
				return e
			}
			m.Files[hex.EncodeToString(fid)] = sha256.Sum256(blob)
		}
		signed, e := v.signManifest(m)
		if e != nil {
			return e
		}
		if e = writeNew(path, signed); e != nil {
			return e
		}
		m.Hash = sha256.Sum256(signed)
		v.manifest = m
		return nil
	}
	if err != nil {
		return err
	}
	m, err := verifyManifest(v.edPublic, v.mlPublic, data)
	if err != nil || m.ScopeID != v.ID() {
		return v.problem(ProblemSignature)
	}
	v.manifest = m
	p, err := v.diskFiles(m)
	if err != nil {
		return err
	}
	nextPath := filepath.Join(v.dir, "manifest.next")
	if p == nil {
		if e := os.Remove(nextPath); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		return nil
	}
	nextData, e := os.ReadFile(nextPath)
	if e == nil {
		next, valid := verifyManifest(v.edPublic, v.mlPublic, nextData)
		if valid == nil && next.ScopeID == v.ID() && next.Generation == m.Generation+1 {
			same, readErr := v.diskFiles(next)
			if readErr != nil {
				return readErr
			}
			if same == nil {
				if err = os.Rename(nextPath, path); err != nil {
					return err
				}
				v.manifest = next
				return nil
			}
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	return p
}

// ready checks that the scope on disk is still the one the manifest describes.
// A leftover manifest.next is harmless here: the next change overwrites it.
func (v *Vault) ready() error {
	if v.nameKey == nil {
		return ErrClosed
	}
	return v.checkDisk()
}
