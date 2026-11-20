package gitread

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/objfile"
	"github.com/go-git/go-git/v5/storage"
)

// boundedStorage reads every object through a cap, so a repository cannot make
// gitread inflate a loose object or decode a commit, tree or blob past
// MaxBytes. A loose object is inflated here, at most MaxBytes+1 bytes, because
// go-git would otherwise trust the size its header states and inflate a lying
// stream whole.
type boundedStorage struct {
	storage.Storer
	fs billy.Filesystem
}

func (s *boundedStorage) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	if obj, ok, err := s.looseObject(t, h); ok {
		return obj, err
	}
	obj, err := s.Storer.EncodedObject(t, h)
	if err != nil {
		return nil, err
	}
	return boundedObject{obj}, nil
}

// looseObject reads a loose object itself, bounded, when the object file
// exists. ok is false when it does not, and the caller then reads a packfile.
func (s *boundedStorage) looseObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, bool, error) {
	hash := h.String()
	name := filepath.Join("objects", hash[:2], hash[2:])
	f, err := s.fs.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, nil
	}
	defer f.Close()
	rd, err := objfile.NewReader(f)
	if err != nil {
		return nil, true, err
	}
	defer rd.Close()
	typ, size, err := rd.Header()
	if err != nil {
		return nil, true, err
	}
	if t != plumbing.AnyObject && typ != t {
		return nil, true, plumbing.ErrObjectNotFound
	}
	if size > MaxBytes {
		return nil, true, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(rd, MaxBytes+1))
	if err != nil {
		return nil, true, err
	}
	if len(body) > MaxBytes {
		return nil, true, ErrTooLarge
	}
	obj := &plumbing.MemoryObject{}
	obj.SetType(typ)
	obj.SetSize(int64(len(body)))
	w, err := obj.Writer()
	if err != nil {
		return nil, true, err
	}
	if _, err := w.Write(body); err != nil {
		return nil, true, err
	}
	if err := w.Close(); err != nil {
		return nil, true, err
	}
	return obj, true, nil
}

// boundedObject caps a packed object's read at MaxBytes+1 and returns
// ErrTooLarge if the stream passes MaxBytes, so a commit, tree or blob cannot
// decode whole.
type boundedObject struct {
	plumbing.EncodedObject
}

func (o boundedObject) Reader() (io.ReadCloser, error) {
	rd, err := o.EncodedObject.Reader()
	if err != nil {
		return nil, err
	}
	return &cappedReader{r: rd}, nil
}

type cappedReader struct {
	r io.ReadCloser
	n int64
}

func (c *cappedReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.n > MaxBytes {
		return 0, ErrTooLarge
	}
	return n, err
}

func (c *cappedReader) Close() error { return c.r.Close() }
