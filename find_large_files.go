package main

import (
	"context"
	"fmt"
	"log"
	"os"

	_ "github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configmap"
)

func main() {
	accessKey := os.Getenv("R2_ACCESS_KEY_ID")
	secretKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	endpoint := os.Getenv("R2_ENDPOINT")

	if accessKey == "" || secretKey == "" || endpoint == "" {
		log.Fatal("R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, or R2_ENDPOINT not set")
	}

	config := configmap.Simple{
		"provider":          "Cloudflare",
		"access_key_id":     accessKey,
		"secret_access_key": secretKey,
		"endpoint":          endpoint,
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
