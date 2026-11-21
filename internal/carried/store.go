package carried

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/panphora/htmlclay/internal/htmlutil"
)

// Store keeps every program text HTML Clay ran, read only, named by the
// SHA-256 of its bytes. A program runs only from the copy read back from
// here, after its bytes hash to the name it was checked under, so nothing the
// page or the file does after the check changes what runs.
type Store struct{ dir string }

// OpenStore opens (making it when missing) the store in dir.
func OpenStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Hash is the name a program text is stored under.
func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func (s *Store) path(hash string) string { return filepath.Join(s.dir, hash+".js") }

var errStoredCopy = errors.New("carried: the stored copy does not match its name")

// Put stores text and returns its hash. A copy already there that still
// matches is kept; one that does not is replaced.
func (s *Store) Put(text string) (string, error) {
	hash := Hash(text)
	if got, err := s.Read(hash); err == nil && got == text {
		return hash, nil
	}
	final := s.path(hash)
	_ = os.Chmod(final, 0o600)
	_ = os.Remove(final)
	tmp, err := os.CreateTemp(s.dir, ".put-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(name, 0o400); err != nil {
		return "", err
	}
	if err := os.Rename(name, final); err != nil {
		if got, rerr := s.Read(hash); rerr == nil && got == text {
			return hash, nil
		}
		return "", err
	}
	return hash, nil
}

// Read returns the stored copy named hash. A copy that is not a plain file,
// is larger than a program may be, or no longer hashes to its name is
// refused, never run.
func (s *Store) Read(hash string) (string, error) {
	if len(hash) != sha256.Size*2 {
		return "", fs.ErrNotExist
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", fs.ErrNotExist
	}
	info, err := os.Lstat(s.path(hash))
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > htmlutil.MaxProgramBytes {
		return "", errStoredCopy
	}
	data, err := os.ReadFile(s.path(hash))
	if err != nil {
		return "", err
	}
	if Hash(string(data)) != hash {
		return "", errStoredCopy
	}
	return string(data), nil
}

// Keep stores every module of a run and reads each back, so the texts it
// returns are the stored copies, checked against their names.
func (s *Store) Keep(modules map[string]string) (map[string]string, error) {
	kept := make(map[string]string, len(modules))
	for name, text := range modules {
		hash, err := s.Put(text)
		if err != nil {
			return nil, err
		}
		stored, err := s.Read(hash)
		if err != nil {
			return nil, err
		}
		kept[name] = stored
	}
	return kept, nil
}
