package blobfs_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"gocloud.dev/blob"
	_ "gocloud.dev/blob/fileblob"

	"github.com/ichizero/blobfs"
)

func Example_fileblob() {
	ctx := context.Background()
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	bucket, err := blob.OpenBucket(ctx, fmt.Sprintf("file://%s/testdata", dir))
	if err != nil {
		log.Fatal(err)
	}

	fsys := blobfs.New(bucket)
	f, err := fsys.Open("foo.txt")
	b, err := io.ReadAll(f)
	if err != nil {
		if err != io.EOF {
			log.Fatal(err)
		}
	}
	log.Print(string(b))
}
