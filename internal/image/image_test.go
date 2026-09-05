package image

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestList_ReturnsEmptyOnEmptyDir(t *testing.T) {
	tempDir := t.TempDir()
	store := New(filepath.Join(tempDir, "images"))

	images, err := store.List()
	if err != nil {
		t.Fatalf("expected nil error on empty dir, got: %v", err)
	}
	if len(images) != 0 {
		t.Errorf("expected empty list, got: %v", images)
	}
}

func TestList_ReturnsSortedImageNames(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	// Create dummy images directly in baseDir
	for _, name := range []string{"ubuntu", "alpine", "debian"} {
		rootfs := filepath.Join(baseDir, name, rootfsDirName)
		if err := os.MkdirAll(rootfs, 0755); err != nil {
			t.Fatalf("failed to create dummy rootfs for %s: %v", name, err)
		}
	}

	images, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	expected := []string{"alpine", "debian", "ubuntu"}
	if !reflect.DeepEqual(images, expected) {
		t.Errorf("expected sorted images %v, got %v", expected, images)
	}
}

func TestGet_ReturnsRootfsPath(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	imageName := "alpine-base"
	expectedRootfs := filepath.Join(baseDir, imageName, rootfsDirName)
	if err := os.MkdirAll(expectedRootfs, 0755); err != nil {
		t.Fatalf("failed creating image dir: %v", err)
	}

	path, err := store.Get(imageName)
	if err != nil {
		t.Fatalf("expected Get to succeed, got error: %v", err)
	}
	if path != expectedRootfs {
		t.Errorf("expected rootfs path %q, got %q", expectedRootfs, path)
	}
}

func TestGet_ErrorOnMissingImage(t *testing.T) {
	tempDir := t.TempDir()
	store := New(filepath.Join(tempDir, "images"))

	path, err := store.Get("non-existent")
	if err == nil {
		t.Fatalf("expected error for missing image, got path: %q", path)
	}
	if !errors.Is(err, ErrImageNotFound) {
		t.Errorf("expected ErrImageNotFound, got: %v", err)
	}
}

func TestImport_CreatesDirStructure(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	// Create a dummy source rootfs
	srcDir := filepath.Join(tempDir, "source-rootfs")
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatalf("failed creating src bin dir: %v", err)
	}
	testFile := filepath.Join(srcDir, "bin", "sh")
	if err := os.WriteFile(testFile, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("failed creating test file: %v", err)
	}

	imageName := "test-image"
	if err := store.Import(imageName, srcDir); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify rootfs structure exists
	destFile := filepath.Join(baseDir, imageName, rootfsDirName, "bin", "sh")
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("expected imported file to exist: %v", err)
	}
	if string(content) != "#!/bin/sh\necho hello\n" {
		t.Errorf("file content mismatch: got %q", string(content))
	}

	// Verify metadata.json
	metaPath := filepath.Join(baseDir, imageName, metadataFileName)
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("expected metadata.json to exist: %v", err)
	}
	var meta Metadata
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("failed to unmarshal metadata.json: %v", err)
	}
	if meta.Name != imageName {
		t.Errorf("expected metadata name %q, got %q", imageName, meta.Name)
	}
	if meta.Created == "" {
		t.Error("expected non-empty created timestamp in metadata")
	}
}

func TestImport_CopiesSubdirectoriesAndFiles(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	srcDir := filepath.Join(tempDir, "complex-rootfs")
	subDirs := []string{"etc", "usr/local/bin", "var/log"}
	for _, d := range subDirs {
		if err := os.MkdirAll(filepath.Join(srcDir, d), 0755); err != nil {
			t.Fatalf("failed creating dir %s: %v", d, err)
		}
	}

	file1 := filepath.Join(srcDir, "etc", "hosts")
	if err := os.WriteFile(file1, []byte("127.0.0.1 localhost"), 0644); err != nil {
		t.Fatalf("writing file1: %v", err)
	}
	file2 := filepath.Join(srcDir, "usr/local/bin", "app")
	if err := os.WriteFile(file2, []byte("binary data"), 0755); err != nil {
		t.Fatalf("writing file2: %v", err)
	}

	if err := store.Import("complex-img", srcDir); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify all files copied
	copiedHosts, err := os.ReadFile(filepath.Join(baseDir, "complex-img", rootfsDirName, "etc", "hosts"))
	if err != nil {
		t.Fatalf("reading copied hosts: %v", err)
	}
	if string(copiedHosts) != "127.0.0.1 localhost" {
		t.Errorf("content mismatch for hosts: %q", string(copiedHosts))
	}

	copiedApp, err := os.ReadFile(filepath.Join(baseDir, "complex-img", rootfsDirName, "usr/local/bin", "app"))
	if err != nil {
		t.Fatalf("reading copied app: %v", err)
	}
	if string(copiedApp) != "binary data" {
		t.Errorf("content mismatch for app: %q", string(copiedApp))
	}
}

func TestImport_EmptyNameError(t *testing.T) {
	tempDir := t.TempDir()
	store := New(filepath.Join(tempDir, "images"))

	err := store.Import("", tempDir)
	if err == nil {
		t.Fatal("expected error when importing with empty name, got nil")
	}
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("expected ErrInvalidName, got: %v", err)
	}
}

func TestImport_MissingSourceError(t *testing.T) {
	tempDir := t.TempDir()
	store := New(filepath.Join(tempDir, "images"))

	err := store.Import("alpine", filepath.Join(tempDir, "does-not-exist"))
	if err == nil {
		t.Fatal("expected error when importing non-existent source path, got nil")
	}
}

func TestImport_AlreadyExistsError(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := store.Import("img1", srcDir); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	err := store.Import("img1", srcDir)
	if err == nil {
		t.Fatal("expected error on duplicate import, got nil")
	}
	if !errors.Is(err, ErrImageExists) {
		t.Errorf("expected ErrImageExists, got: %v", err)
	}
}

func TestRemove_DeletesImageDir(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	imageDir := filepath.Join(baseDir, "to-delete", rootfsDirName)
	if err := os.MkdirAll(imageDir, 0755); err != nil {
		t.Fatalf("creating image dir: %v", err)
	}

	if err := store.Remove("to-delete"); err != nil {
		t.Fatalf("expected Remove to succeed, got: %v", err)
	}

	if _, err := os.Stat(filepath.Join(baseDir, "to-delete")); !os.IsNotExist(err) {
		t.Errorf("expected image directory to be removed, stat returned: %v", err)
	}
}

func TestRemove_MissingImageError(t *testing.T) {
	tempDir := t.TempDir()
	store := New(filepath.Join(tempDir, "images"))

	err := store.Remove("does-not-exist")
	if err == nil {
		t.Fatal("expected error removing non-existent image, got nil")
	}
	if !errors.Is(err, ErrImageNotFound) {
		t.Errorf("expected ErrImageNotFound, got: %v", err)
	}
}

func TestImport_CopiesSymlinks(t *testing.T) {
	tempDir := t.TempDir()
	baseDir := filepath.Join(tempDir, "images")
	store := New(baseDir)

	srcDir := filepath.Join(tempDir, "symlink-src")
	if err := os.MkdirAll(filepath.Join(srcDir, "bin"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	shPath := filepath.Join(srcDir, "bin", "sh")
	if err := os.WriteFile(shPath, []byte("#!/bin/sh"), 0755); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Create symlink bin/bash -> sh
	bashLink := filepath.Join(srcDir, "bin", "bash")
	if err := os.Symlink("sh", bashLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := store.Import("symlink-img", srcDir); err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	destLink := filepath.Join(baseDir, "symlink-img", rootfsDirName, "bin", "bash")
	info, err := os.Lstat(destLink)
	if err != nil {
		t.Fatalf("lstat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected destination to be a symlink")
	}

	target, err := os.Readlink(destLink)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "sh" {
		t.Errorf("expected symlink target 'sh', got %q", target)
	}
}

func TestValidateImageName_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	store := New(filepath.Join(tempDir, "images"))

	invalidNames := []string{"", "  ", "../escape", "foo/bar", "foo\\bar"}
	for _, name := range invalidNames {
		if _, err := store.Get(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Get(%q): expected ErrInvalidName, got: %v", name, err)
		}
		if err := store.Import(name, tempDir); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Import(%q): expected ErrInvalidName, got: %v", name, err)
		}
		if err := store.Remove(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Remove(%q): expected ErrInvalidName, got: %v", name, err)
		}
	}
}

func TestNewDefault(t *testing.T) {
	store := NewDefault()
	if store == nil {
		t.Fatal("expected non-nil Store from NewDefault()")
	}
}

