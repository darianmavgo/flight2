package main

import (
	"context"
	"fmt"
	"log"

	_ "github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
)

func main() {
	config := configmap.Simple{
		"provider":          "Cloudflare",
		"access_key_id":     "0d5aacd854377d79f3c83caa688effbe",
		"secret_access_key": "986a762b395b7b9ebc6c08a62a64cbd8a872654ce7c927270e46cab19c9b0af5",
		"endpoint":          "https://d8dc30936fb37cbd74552d31a709f6cf.r2.cloudflarestorage.com",
		"region":            "auto",
		"type":              "s3",
		"chunk_size":        "5Mi",
		"copy_cutoff":       "5Mi",
	}
	reg, err := fs.Find("s3")
	if err != nil {
		log.Fatal(err)
	}
	fsrc, err := reg.NewFs(context.Background(), "r2", "test-mksqlite/sample_data", config)
	if err != nil {
		log.Fatal(err)
	}
	entries, err := fsrc.List(context.Background(), "")
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		if obj, ok := entry.(fs.Object); ok {
			fmt.Printf("%s: %d bytes\n", obj.Remote(), obj.Size())
		}
	}
}
