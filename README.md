# blobfs

[![test](https://github.com/ichizero/blobfs/actions/workflows/test.yml/badge.svg)](https://github.com/ichizero/blobfs/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ichizero/blobfs.svg)](https://pkg.go.dev/github.com/ichizero/blobfs)
[![Codecov](https://codecov.io/gh/ichizero/blobfs/branch/main/graph/badge.svg)](https://codecov.io/gh/ichizero/blobfs)
[![Go Report Card](https://goreportcard.com/badge/github.com/ichizero/blobfs)](https://goreportcard.com/report/github.com/ichizero/blobfs)

Package blobfs provides access with fs.FS interface to files stored in a blob storage.

FS implements fs.FS, so it can be used with any packages that understands file system interfaces.

e.g.) net/http, text/template, html/template

It uses gocloud.dev/blob for a blob backend, so it can read the following blob storages.

- Google Cloud Storage
- Amazon S3
- Azure Blob Storage
- Local filesystem
- In memory filesystem

For more details about gocloud.dev/blob, please refer to the following page.
ref.) https://gocloud.dev/howto/blob/

## Example

```go
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
```
