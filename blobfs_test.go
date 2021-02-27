package blobfs_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"testing/fstest"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"

	"github.com/ichizero/blobfs"
)

func TestFS(t *testing.T) {
	ctx := context.Background()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bucket, err := blob.OpenBucket(ctx, fmt.Sprintf("file://%s/testdata", dir))
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"foo.txt",
		"bar.txt",
		"dir1",
		"dir1/hoge.txt",
		"dir1/dir1-1",
		"dir1/dir1-1/fuga.txt",
		"dir2",
		"dir2/hello.txt",
	}

	fsys := blobfs.New(bucket)
	if err := fstest.TestFS(fsys, expected...); err != nil {
		t.Fatal(err)
	}
}
