package image

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// fsOps abstracts filesystem operations for localImageStore to support testability.
type fsOps interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(name string, data []byte, perm fs.FileMode) error
	RemoveAll(path string) error
	ReadDir(name string) ([]os.DirEntry, error)
	Stat(name string) (os.FileInfo, error)
	CopyDir(src, dst string) error
}

// realFs implements fsOps using actual OS calls.
type realFs struct{}

func (realFs) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (realFs) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (realFs) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (realFs) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (realFs) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

// CopyDir recursively copies all files, directories, and symlinks from src to dst.
func (realFs) CopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("calculating relative path: %w", err)
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("reading symlink %s: %w", path, err)
			}
			return os.Symlink(linkTarget, targetPath)
		}

		// Handle regular files
		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening source file %s: %w", path, err)
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return fmt.Errorf("creating destination file %s: %w", targetPath, err)
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("copying content to %s: %w", targetPath, err)
		}

		return nil
	})
}

// localImageStore implements image.Store backed by a local filesystem directory.
type localImageStore struct {
	baseDir string
	fs      fsOps
}

// newLocalStore creates a localImageStore with the default realFs.
func newLocalStore(baseDir string) *localImageStore {
	return newLocalStoreWith(baseDir, realFs{})
}

// newLocalStoreWith creates a localImageStore with an injected fsOps.
func newLocalStoreWith(baseDir string, fs fsOps) *localImageStore {
	return &localImageStore{
		baseDir: baseDir,
		fs:      fs,
	}
}

// List returns all available image names in alphabetical order.
func (s *localImageStore) List() ([]string, error) {
	entries, err := s.fs.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading image directory %s: %w", s.baseDir, err)
	}

	images := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check that rootfs subdirectory exists inside the image directory
		rootfsPath := filepath.Join(s.baseDir, entry.Name(), rootfsDirName)
		if info, err := s.fs.Stat(rootfsPath); err == nil && info.IsDir() {
			images = append(images, entry.Name())
		}
	}

	sort.Strings(images)
	return images, nil
}

// Get returns the rootfs path for a named image.
func (s *localImageStore) Get(name string) (string, error) {
	if err := validateImageName(name); err != nil {
		return "", err
	}

	rootfsPath := filepath.Join(s.baseDir, name, rootfsDirName)
	info, err := s.fs.Stat(rootfsPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: image %q not found", ErrImageNotFound, name)
	}

	return rootfsPath, nil
}

// Import copies a directory into the image store as a new named image.
func (s *localImageStore) Import(name, srcPath string) error {
	if err := validateImageName(name); err != nil {
		return err
	}

	if srcPath == "" {
		return fmt.Errorf("source path cannot be empty")
	}

	if _, err := s.fs.Stat(srcPath); err != nil {
		return fmt.Errorf("source path invalid: %w", err)
	}

	imageDir := filepath.Join(s.baseDir, name)
	if _, err := s.fs.Stat(imageDir); err == nil {
		return fmt.Errorf("%w: image %q already exists", ErrImageExists, name)
	}

	targetRootfs := filepath.Join(imageDir, rootfsDirName)
	if err := s.fs.MkdirAll(targetRootfs, 0755); err != nil {
		return fmt.Errorf("creating image rootfs directory: %w", err)
	}

	if err := s.fs.CopyDir(srcPath, targetRootfs); err != nil {
		_ = s.fs.RemoveAll(imageDir)
		return fmt.Errorf("copying rootfs from %s: %w", srcPath, err)
	}

	meta := Metadata{
		Name:    name,
		Created: time.Now().UTC().Format(time.RFC3339),
	}

	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		_ = s.fs.RemoveAll(imageDir)
		return fmt.Errorf("marshaling image metadata: %w", err)
	}

	metaFile := filepath.Join(imageDir, metadataFileName)
	if err := s.fs.WriteFile(metaFile, metaBytes, 0644); err != nil {
		_ = s.fs.RemoveAll(imageDir)
		return fmt.Errorf("writing metadata.json: %w", err)
	}

	return nil
}

// Remove deletes an image from the store.
func (s *localImageStore) Remove(name string) error {
	if err := validateImageName(name); err != nil {
		return err
	}

	imageDir := filepath.Join(s.baseDir, name)
	if _, err := s.fs.Stat(imageDir); os.IsNotExist(err) {
		return fmt.Errorf("%w: image %q not found", ErrImageNotFound, name)
	}

	if err := s.fs.RemoveAll(imageDir); err != nil {
		return fmt.Errorf("removing image directory %s: %w", imageDir, err)
	}

	return nil
}

func validateImageName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidName)
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("%w: name cannot contain path traversal characters", ErrInvalidName)
	}
	return nil
}
