package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type cleanupFailure struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}
type cleanupResult struct {
	Removed  int              `json:"removed"`
	Skipped  int              `json:"skipped"`
	Failures []cleanupFailure `json:"failures"`
}

// Walk directory descriptors without following symlinks, including the root.
// Never recursively remove a folder or accept a file path supplied by the caller.
func taskFile(root, relative string, expectedSize *int64, remove bool) error {
	if !filepath.IsAbs(root) {
		return errors.New("下载目录无效")
	}
	if _, err := safeRelative(relative); err != nil {
		return err
	}
	fd, err := unix.Open("/", cleanupDirectoryOpenFlags, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(root), "/"), "/")
	parts = append(parts, strings.Split(relative, "/")...)
	for _, part := range parts[:len(parts)-1] {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, cleanupDirectoryOpenFlags, 0)
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		if err != nil {
			if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
				return errors.New("无权访问下载目录，请检查应用的目录授权")
			}
			if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
				return errors.New("下载路径包含符号链接或非目录项")
			}
			return fmt.Errorf("下载目录不可访问：%w", err)
		}
		_ = unix.Close(fd)
		fd = next
	}
	name := parts[len(parts)-1]
	var stat unix.Stat_t
	if err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("目标不是普通文件，已保留记录")
	}
	if expectedSize != nil && stat.Size != *expectedSize {
		return errors.New("文件大小已变化，已保留记录")
	}
	if !remove {
		return nil
	}
	if err := unix.Unlinkat(fd, name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
			return errors.New("没有删除文件的权限，请检查应用对下载目录的读写授权")
		}
		return err
	}
	return nil
}

func (e *Engine) cleanupTasks(ids []string, deleteFiles bool, allowed []string) (cleanupResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store.mu.Lock()
	defer e.store.mu.Unlock()
	result := cleanupResult{Failures: []cleanupFailure{}}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	old := e.store.state.Tasks
	kept := make([]*Task, 0, len(old))
	// Fail before deleting files if the state store is already unwritable.
	if deleteFiles {
		if err := e.store.save(); err != nil {
			return result, err
		}
	}
	for _, t := range old {
		if !selected[t.ID] {
			kept = append(kept, t)
			continue
		}
		delete(selected, t.ID)
		if (t.Status != "COMPLETED" && t.Status != "FAILED") || e.running[t.ID] != nil {
			kept = append(kept, t)
			result.Skipped++
			continue
		}
		if deleteFiles {
			root := t.Destination
			if root == "" {
				root = e.store.state.Settings.DownloadDir
			}
			authorized := false
			for _, path := range allowed {
				if filepath.IsAbs(root) && filepath.Clean(root) == filepath.Clean(path) {
					authorized = true
					break
				}
			}
			var err error
			if !authorized {
				err = errors.New("下载目录已不在 fnOS 授权范围内")
			} else {
				err = e.removeTaskFiles(t, root, old)
			}
			if err != nil {
				kept = append(kept, t)
				result.Failures = append(result.Failures, cleanupFailure{ID: t.ID, Message: err.Error()})
				continue
			}
		}
		result.Removed++
	}
	result.Skipped += len(selected)
	if result.Removed == 0 {
		return result, nil
	}
	e.store.state.Tasks = kept
	if err := e.store.save(); err != nil {
		e.store.state.Tasks = old
		if deleteFiles {
			return cleanupResult{}, fmt.Errorf("部分文件可能已删除，但记录保存失败；任务记录已保留，请刷新后重试：%w", err)
		}
		return cleanupResult{}, err
	}
	return result, nil
}

func (e *Engine) removeTaskFiles(t *Task, root string, tasks []*Task) error {
	if strings.ContainsAny(t.ID, "/\\") || t.ID == "" || t.ID == "." || t.ID == ".." {
		return errors.New("任务标识无效")
	}
	type file struct {
		relative string
		size     *int64
	}
	files := []file{{".quarkdrive-part/" + t.ID + ".part", nil}}
	// Failed tasks own only their ID-based partial file; a conflicting final file may belong to the user.
	if t.Status == "COMPLETED" {
		for _, other := range tasks {
			otherRoot := other.Destination
			if otherRoot == "" {
				otherRoot = e.store.state.Settings.DownloadDir
			}
			if other.ID != t.ID && filepath.Clean(otherRoot) == filepath.Clean(root) && other.Relative == t.Relative {
				return errors.New("文件仍被其他任务引用")
			}
		}
		files = append(files, file{t.Relative, &t.Size})
	}
	// Validate every target before any deletion. Recheck when deleting as paths may change.
	for _, remove := range []bool{false, true} {
		for _, f := range files {
			if err := taskFile(root, f.relative, f.size, remove); err != nil {
				return err
			}
		}
	}
	return nil
}

// Records-only compatibility helper; this path never touches local files.
func (e *Engine) clearRecords(ids []string) (int, error) {
	result, err := e.cleanupTasks(ids, false, nil)
	return result.Removed, err
}
