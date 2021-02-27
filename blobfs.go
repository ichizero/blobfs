// Package blobfs provides access to files stored in a blob storage.
//
// FS implements fs.FS, so it can be used with any packages that understands file system interfaces.
// e.g.) net/http, text/template, html/template
package blobfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"gocloud.dev/blob"
)

const (
	opOpen = "open"
	opRead = "read"
	opSeek = "seek"
)

// An FS is a read-only blob storage file system that implements fs.FS interface.
type FS struct {
	bucket *blob.Bucket
}

var (
	_ fs.ReadFileFS = (*FS)(nil)
	_ fs.ReadDirFS  = (*FS)(nil)
)

// New returns FS object that can interact with a blob storage.
func New(bucket *blob.Bucket) *FS {
	return &FS{bucket: bucket}
}

var rootFile = &fileListEntry{
	obj: &blob.ListObject{
		Key:     "",
		ModTime: time.Time{},
		Size:    0,
		MD5:     nil,
		IsDir:   true,
	},
}

func (fsys *FS) searchDir(ctx context.Context, name string) (*fileListEntry, error) {
	name = strings.TrimSuffix(name, "/")
	iter := fsys.bucket.List(&blob.ListOptions{
		Delimiter: "/",
		Prefix:    name,
	})
	obj, err := iter.Next(ctx)
	if err == io.EOF {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	if obj.Key == name+"/" {
		return &fileListEntry{obj: obj}, nil
	}
	return nil, fs.ErrNotExist
}

func (fsys *FS) lookup(ctx context.Context, name string) (*fileListEntry, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	if name == "." {
		return rootFile, nil
	}
	attr, err := fsys.bucket.Attributes(ctx, name)
	if err == nil {
		return &fileListEntry{obj: &blob.ListObject{
			Key:     name,
			ModTime: attr.ModTime,
			Size:    attr.Size,
			MD5:     attr.MD5,
			IsDir:   false,
		}}, err
	}
	return fsys.searchDir(ctx, name)
}

// Open opens the named file for reading and returns it as an fs.File.
func (fsys *FS) Open(name string) (fs.File, error) {
	ctx := context.TODO()
	e, err := fsys.lookup(ctx, name)
	if err != nil {
		return nil, &fs.PathError{Op: opOpen, Path: name, Err: err}
	}
	if e.IsDir() {
		dir, err := newOpenDir(ctx, e, fsys.bucket)
		if err != nil {
			return nil, &fs.PathError{Op: opOpen, Path: name, Err: err}
		}
		return dir, nil
	}
	file, err := newOpenFile(e, fsys.bucket)
	if err != nil {
		return nil, &fs.PathError{Op: opOpen, Path: name, Err: err}
	}
	return file, nil
}

// ReadFile reads and returns the content of the named file.
func (fsys *FS) ReadFile(name string) ([]byte, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	ofile, ok := file.(*openFile)
	if !ok {
		return nil, &fs.PathError{Op: opRead, Path: name, Err: errors.New("is a directory")}
	}

	b := make([]byte, ofile.self.Size())
	_, err = ofile.ReadAt(b, 0)
	if err != io.EOF {
		return nil, err
	}
	return b, nil
}

// ReadDir reads and returns all entries in the named directory.
func (fsys *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	dir, ok := file.(*openDir)
	if !ok {
		return nil, &fs.PathError{Op: opRead, Path: name, Err: errors.New("not a directory")}
	}
	return dir.ReadDir(-1)
}

type fileListEntry struct {
	obj *blob.ListObject
}

var (
	_ fs.FileInfo = (*fileListEntry)(nil)
	_ fs.DirEntry = (*fileListEntry)(nil)
)

func (e *fileListEntry) Name() string {
	if e.obj.Key == "" {
		return "."
	}
	name := e.obj.Key
	if name[len(name)-1] == '/' {
		name = name[:len(name)-1]
	}
	slice := strings.Split(name, "/")
	return slice[len(slice)-1]
}

func (e *fileListEntry) Size() int64                { return e.obj.Size }
func (e *fileListEntry) ModTime() time.Time         { return e.obj.ModTime }
func (e *fileListEntry) IsDir() bool                { return e.obj.IsDir }
func (e *fileListEntry) Sys() interface{}           { return nil }
func (e *fileListEntry) Type() fs.FileMode          { return e.Mode().Type() }
func (e *fileListEntry) Info() (fs.FileInfo, error) { return e, nil }

func (e *fileListEntry) Mode() fs.FileMode {
	if e.IsDir() {
		return fs.ModeDir | 0555
	}
	return 0444
}

func (e *fileListEntry) Path() string { return e.obj.Key }

type openFile struct {
	self   *fileListEntry
	bucket *blob.Bucket
	offset int64
}

var (
	_ fs.File     = (*openFile)(nil)
	_ io.ReaderAt = (*openFile)(nil)
	_ io.Seeker   = (*openFile)(nil)
)

func newOpenFile(entry *fileListEntry, bucket *blob.Bucket) (*openFile, error) {
	return &openFile{
		self:   entry,
		bucket: bucket,
		offset: 0,
	}, nil
}

func (f *openFile) Stat() (fs.FileInfo, error) { return f.self, nil }
func (f *openFile) Close() error               { return nil }

func (f *openFile) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if f.offset >= f.self.Size() {
		return 0, io.EOF
	}

	ctx := context.TODO()
	r, err := f.bucket.NewRangeReader(ctx, f.self.Path(), f.offset, int64(len(b)), nil)
	if err != nil {
		return 0, &fs.PathError{Op: opRead, Path: f.self.Path(), Err: err}
	}
	size, err := r.Read(b)
	f.offset += int64(size)
	if err != nil {
		return size, err
	}
	return size, r.Close()
}

func (f *openFile) ReadAt(b []byte, offset int64) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if offset >= f.self.Size() {
		return 0, io.EOF
	}

	ctx := context.TODO()
	r, err := f.bucket.NewRangeReader(ctx, f.self.Path(), offset, int64(len(b)), nil)
	if err != nil {
		return 0, &fs.PathError{Op: opRead, Path: f.self.Path(), Err: err}
	}
	size, err := r.Read(b)
	if offset+int64(size) == f.self.Size() {
		return size, io.EOF
	}
	if err != nil {
		return size, err
	}
	return size, r.Close()
}

func (f *openFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	default:
		return 0, &fs.PathError{Op: opSeek, Path: f.self.Path(), Err: fs.ErrInvalid}
	case io.SeekStart:
		// offset += 0
	case io.SeekCurrent:
		offset += f.offset
	case io.SeekEnd:
		offset += f.self.Size()
	}
	if offset < 0 || offset > f.self.Size() {
		return 0, &fs.PathError{Op: opSeek, Path: f.self.Path(), Err: fs.ErrInvalid}
	}
	f.offset = offset
	return offset, nil
}

type openDir struct {
	self    *fileListEntry
	entries []*fileListEntry
	offset  int
}

var (
	_ fs.ReadDirFile = (*openDir)(nil)
)

func newOpenDir(ctx context.Context, entry *fileListEntry, bucket *blob.Bucket) (*openDir, error) {
	iter := bucket.List(&blob.ListOptions{
		Delimiter: "/",
		Prefix:    entry.Path(),
	})
	files := make([]*fileListEntry, 0)
	for {
		obj, err := iter.Next(ctx)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		files = append(files, &fileListEntry{obj: obj})
	}
	return &openDir{self: entry, entries: files}, nil
}

func (d *openDir) Stat() (fs.FileInfo, error) { return d.self, nil }
func (d *openDir) Close() error               { return nil }

func (d *openDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: opRead, Path: d.self.Path(), Err: errors.New("is a directory")}
}

func (d *openDir) ReadDir(n int) ([]fs.DirEntry, error) {
	count := len(d.entries) - d.offset
	if n > 0 && count > n {
		count = n
	}
	if count == 0 {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	entries := make([]fs.DirEntry, count)
	for i := range entries {
		entries[i] = d.entries[d.offset+i]
	}
	d.offset += count
	return entries, nil
}
