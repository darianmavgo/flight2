# Rclone Features Recommendation for Flight2

After scanning the `flight2` codebase and the `rclone` repository, I have identified several key features from `rclone` that `flight2` should adopt.

## 1. Adopt the `vfs` Package (Strongly Recommended)

Currently, `flight2` interacts with rclone backends directly using the `fs` interface.

### Why use `vfs`?
The `vfs` package (`github.com/rclone/rclone/vfs`) wraps an `fs.Fs` and provides an OS-like filesystem interface.

*   **Directory Caching**: `vfs` automatically caches directory listings, significantly speeding up browsing for slower remotes (S3, Drive).
*   **File Handle Management**: It provides `Open`, `Create`, `Rename`, `Remove` methods.
*   **VFS Caching**: It supports VFS file caching (`--vfs-cache-mode`), enabling seeking in files that don't natively support it and safe writes.

### Implementation Strategy
In `flight2/internal/dataset_source/source.go`:
1.  Instead of returning `fs.Fs`, wrap it with `vfs.New(fsrc, vfsOpt)`.
2.  Store the `*vfs.VFS` instance.
3.  Use `vfs.Stat(path)` and `vfs.Open(path)` instead of `fsrc.NewObject`.

## 2. Leverage `http.ServeContent` with VFS Handles

`vfs.File` handles are compatible with `http.ServeContent`, allowing for:
*   **Range Requests**: Essential for video seeking and resuming downloads.
*   Standard library compatibility.

## 3. High-Level Operations (`fs/operations`)

For future file management features (move/copy/sync), use `github.com/rclone/rclone/fs/operations` instead of implementing manual copy logic.

## 4. Configuration Compatibility (`fs/config`)

Consider using `github.com/rclone/rclone/fs/config/configfile` to parse standard `rclone.conf` files, allowing easier migration for users.
