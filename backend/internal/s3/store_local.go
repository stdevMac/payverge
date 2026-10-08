package s3

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Local driver layout under one root (STORAGE_DIR/public or /protected):
//
//	objects/<key>       the bytes (0600)
//	meta/<key>.json     content type, disposition, user metadata, sha256 ETag
//	tmp/                same-filesystem staging for atomic temp+rename writes
//
// Directories are 0700. The tree is plain files, so backing up the volume is a
// tar/rsync of STORAGE_DIR (docs/self-hosting/storage.md).
//
// Every file operation resolves through an os.Root opened on the store root,
// so a symlink planted in the tree (a restored backup, another process with
// volume access) can never make the store read, write or delete outside it.
// The public root cannot reach the protected one this way either. The root
// directory itself may be a symlink or mount point.
const (
	localObjectsDir = "objects"
	localMetaDir    = "meta"
	localTmpDir     = "tmp"
	localDirPerm    = 0o700
	localFilePerm   = 0o600
)

// LocalStore is the zero-dependency default driver.
type LocalStore struct {
	root    string
	fsys    *os.Root // writes (temp + rename spans tmp/ and objects/)
	objects *os.Root // objects/, for reads and deletes
	meta    *os.Root // meta/, for sidecar reads and deletes
}

// localMeta is the sidecar persisted next to each object.
type localMeta struct {
	ContentType        string            `json:"content_type,omitempty"`
	ContentDisposition string            `json:"content_disposition,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	SHA256             string            `json:"sha256"`
	Size               int64             `json:"size"`
}

// NewLocalStore opens (creating if needed) a local store rooted at root.
// Reopening the same root sees every object written before — there is no
// in-memory index.
func NewLocalStore(root string) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("storage: local root directory is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve %q: %w", root, err)
	}
	if err := os.MkdirAll(abs, localDirPerm); err != nil {
		return nil, fmt.Errorf("storage: create %s: %w", abs, err)
	}
	fsys, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", abs, err)
	}
	s := &LocalStore{root: abs, fsys: fsys}
	fail := func(err error) (*LocalStore, error) {
		s.close()
		return nil, err
	}
	for _, dir := range []string{localObjectsDir, localMetaDir, localTmpDir} {
		if err := fsys.MkdirAll(dir, localDirPerm); err != nil {
			return fail(fmt.Errorf("storage: create %s: %w", filepath.Join(abs, dir), err))
		}
	}
	// Sub-roots for the read path: one fewer component to walk per request.
	if s.objects, err = fsys.OpenRoot(localObjectsDir); err != nil {
		return fail(fmt.Errorf("storage: open %s: %w", filepath.Join(abs, localObjectsDir), err))
	}
	if s.meta, err = fsys.OpenRoot(localMetaDir); err != nil {
		return fail(fmt.Errorf("storage: open %s: %w", filepath.Join(abs, localMetaDir), err))
	}
	// Probe writability now so a read-only volume fails at startup, not on the
	// first upload.
	name, probe, err := s.createTemp(".probe-")
	if err != nil {
		return fail(fmt.Errorf("storage: %s is not writable: %w", filepath.Join(abs, localTmpDir), err))
	}
	_ = probe.Close()
	_ = fsys.Remove(name)
	return s, nil
}

func (s *LocalStore) close() {
	for _, r := range []*os.Root{s.meta, s.objects, s.fsys} {
		if r != nil {
			_ = r.Close()
		}
	}
}

// Root returns the absolute root directory.
func (s *LocalStore) Root() string { return s.root }

func (s *LocalStore) Driver() string { return DriverLocal }

// paths maps a validated key onto its object and meta file names, relative
// to the objects/ and meta/ roots, re-checking that they are local names
// (belt and braces over ValidateKey; os.Root then refuses symlink escapes).
func (s *LocalStore) paths(key string) (objectPath, metaPath string, err error) {
	if err := ValidateKey(key); err != nil {
		return "", "", err
	}
	objectPath = filepath.FromSlash(key)
	metaPath = objectPath + ".json"
	if !filepath.IsLocal(objectPath) || objectPath == "." {
		return "", "", ErrInvalidKey
	}
	return objectPath, metaPath, nil
}

// Put writes body atomically: stream to a temp file in tmp/ (hashing as it
// goes), fsync, rename over objects/<key>, then publish the meta sidecar the
// same way. A reader never sees a partial object.
func (s *LocalStore) Put(ctx context.Context, key string, body io.Reader, opts PutOptions) error {
	objectPath, metaPath, err := s.paths(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if body == nil {
		body = strings.NewReader("")
	}
	hash := sha256.New()
	tmpName, size, err := s.writeTemp(io.TeeReader(&ctxReader{ctx: ctx, r: body}, hash))
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.fsys.Remove(tmpName)
		}
	}()
	objectName := filepath.Join(localObjectsDir, objectPath)
	if err := s.fsys.MkdirAll(filepath.Dir(objectName), localDirPerm); err != nil {
		return fmt.Errorf("storage: create object dir: %w", err)
	}
	if err := s.fsys.Rename(tmpName, objectName); err != nil {
		return fmt.Errorf("storage: commit object: %w", err)
	}
	committed = true

	meta := localMeta{
		ContentType:        strings.TrimSpace(opts.ContentType),
		ContentDisposition: strings.TrimSpace(opts.ContentDisposition),
		Metadata:           copyMetadata(opts.Metadata),
		SHA256:             hex.EncodeToString(hash.Sum(nil)),
		Size:               size,
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("storage: encode meta: %w", err)
	}
	metaTmp, _, err := s.writeTemp(strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	metaName := filepath.Join(localMetaDir, metaPath)
	if err := s.fsys.MkdirAll(filepath.Dir(metaName), localDirPerm); err != nil {
		_ = s.fsys.Remove(metaTmp)
		return fmt.Errorf("storage: create meta dir: %w", err)
	}
	if err := s.fsys.Rename(metaTmp, metaName); err != nil {
		_ = s.fsys.Remove(metaTmp)
		return fmt.Errorf("storage: commit meta: %w", err)
	}
	return nil
}

// createTemp creates a new, exclusive file in tmp/ and returns its root-relative
// name. os.Root has no CreateTemp, so the name carries 64 random bits.
func (s *LocalStore) createTemp(prefix string) (string, *os.File, error) {
	for range 8 {
		var b [8]byte
		_, _ = rand.Read(b[:])
		name := filepath.Join(localTmpDir, prefix+hex.EncodeToString(b[:]))
		f, err := s.fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, localFilePerm)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return name, f, err
	}
	return "", nil, errors.New("storage: no unique temp file name")
}

func (s *LocalStore) writeTemp(r io.Reader) (name string, size int64, err error) {
	name, f, err := s.createTemp("put-")
	if err != nil {
		return "", 0, fmt.Errorf("storage: create temp file: %w", err)
	}
	fail := func(e error) (string, int64, error) {
		_ = f.Close()
		_ = s.fsys.Remove(name)
		return "", 0, e
	}
	if err := f.Chmod(localFilePerm); err != nil {
		return fail(fmt.Errorf("storage: chmod temp file: %w", err))
	}
	size, err = io.Copy(f, r)
	if err != nil {
		return fail(fmt.Errorf("storage: write object: %w", err))
	}
	if err := f.Sync(); err != nil {
		return fail(fmt.Errorf("storage: sync object: %w", err))
	}
	if err := f.Close(); err != nil {
		_ = s.fsys.Remove(name)
		return "", 0, fmt.Errorf("storage: close object: %w", err)
	}
	return name, size, nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (*Object, error) {
	objectPath, metaPath, err := s.paths(key)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := s.objects.Open(objectPath)
	if err != nil {
		return nil, mapFSError(err)
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, mapFSError(err)
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, ErrNotFound
	}
	return &Object{ObjectInfo: s.info(key, fi, metaPath), Body: f}, nil
}

func (s *LocalStore) Stat(ctx context.Context, key string) (*ObjectInfo, error) {
	objectPath, metaPath, err := s.paths(key)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fi, err := s.objects.Stat(objectPath)
	if err != nil {
		return nil, mapFSError(err)
	}
	if !fi.Mode().IsRegular() {
		return nil, ErrNotFound
	}
	info := s.info(key, fi, metaPath)
	return &info, nil
}

func (s *LocalStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Stat(ctx, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Delete removes the object and its sidecar. Missing objects are not an error.
// Empty parent directories are left in place (cheap, and avoids racing a
// concurrent Put into the same folder).
func (s *LocalStore) Delete(ctx context.Context, key string) error {
	objectPath, metaPath, err := s.paths(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fi, statErr := s.objects.Lstat(objectPath)
	if statErr == nil && !fi.Mode().IsRegular() {
		return ErrNotFound
	}
	if statErr != nil && escapesRoot(statErr) {
		return ErrNotFound
	}
	if err := s.objects.Remove(objectPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("storage: delete object: %w", err)
	}
	if err := s.meta.Remove(metaPath); err != nil && !errors.Is(err, fs.ErrNotExist) && !escapesRoot(err) {
		return fmt.Errorf("storage: delete meta: %w", err)
	}
	return nil
}

// info merges the file stat with the sidecar. A missing, unreadable or stale
// sidecar (size mismatch, e.g. a crash between the two renames) degrades to an
// extension-derived content type and a weak size/mtime ETag.
func (s *LocalStore) info(key string, fi os.FileInfo, metaPath string) ObjectInfo {
	info := ObjectInfo{Key: key, Size: fi.Size(), ModTime: fi.ModTime().UTC()}
	var meta localMeta
	if raw, err := s.meta.ReadFile(metaPath); err == nil && json.Unmarshal(raw, &meta) == nil && meta.Size == fi.Size() {
		info.ContentType = meta.ContentType
		info.ContentDisposition = meta.ContentDisposition
		info.Metadata = copyMetadata(meta.Metadata)
		if len(meta.SHA256) >= 32 {
			info.ETag = `"` + meta.SHA256[:32] + `"`
		}
	}
	if info.ContentType == "" {
		info.ContentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(key)))
	}
	if info.ETag == "" {
		info.ETag = `W/"` + strconv.FormatInt(fi.Size(), 16) + "-" + strconv.FormatInt(fi.ModTime().UnixNano(), 16) + `"`
	}
	return info
}

func mapFSError(err error) error {
	if errors.Is(err, fs.ErrNotExist) || escapesRoot(err) {
		return ErrNotFound
	}
	// ENOTDIR: a path component is a regular file ("a" exists, asked "a/b").
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && strings.Contains(strings.ToLower(pathErr.Err.Error()), "not a directory") {
		return ErrNotFound
	}
	return fmt.Errorf("storage: %w", err)
}

// escapesRoot reports an os.Root refusal: a symlink that leaves the store, an
// absolute symlink, or a symlink loop. The store answers these like a missing
// object. os exports no sentinel for the escape case, so it matches the text;
// TestLocalStoreSymlinkEscapes pins it against toolchain changes.
func escapesRoot(err error) bool {
	if errors.Is(err, syscall.ELOOP) {
		return true
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		err = linkErr.Err
	}
	return err != nil && err.Error() == "path escapes from parent"
}

func copyMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// ctxReader aborts a long copy when the request context is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

var _ Store = (*LocalStore)(nil)
