package container

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreate_GeneratesUniqueID(t *testing.T) {
	mockFs := newMockFsWriter()
	store := newContainerStoreWith("/containers", mockFs)

	id1, merged1, err := store.Create("/images/alpine/rootfs")
	if err != nil {
		t.Fatalf("unexpected error creating container 1: %v", err)
	}

	id2, merged2, err := store.Create("/images/alpine/rootfs")
	if err != nil {
		t.Fatalf("unexpected error creating container 2: %v", err)
	}

	if id1 == id2 {
		t.Fatalf("expected unique IDs, got identical %q", id1)
	}

	if len(id1) != 12 || len(id2) != 12 {
		t.Errorf("expected 12-char hex IDs, got len %d and %d", len(id1), len(id2))
	}

	if merged1 == merged2 {
		t.Fatalf("expected unique merged paths, got %q", merged1)
	}
}

func TestCreate_CreatesDirStructure(t *testing.T) {
	mockFs := newMockFsWriter()
	baseDir := "/containers"
	store := newContainerStoreWith(baseDir, mockFs)

	id, merged, err := store.Create("/images/alpine/rootfs")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expectedUpper := filepath.Join(baseDir, id, upperDirName)
	expectedWork := filepath.Join(baseDir, id, workDirName)
	expectedMerged := filepath.Join(baseDir, id, mergedDirName)

	if merged != expectedMerged {
		t.Errorf("expected merged %q, got %q", expectedMerged, merged)
	}

	// Verify all 3 subdirectories were created
	hasUpper, hasWork, hasMerged := false, false, false
	for _, call := range mockFs.mkdirAllCalls {
		if call == expectedUpper {
			hasUpper = true
		}
		if call == expectedWork {
			hasWork = true
		}
		if call == expectedMerged {
			hasMerged = true
		}
	}

	if !hasUpper || !hasWork || !hasMerged {
		t.Errorf("expected upper, work, and merged to be created in %v", mockFs.mkdirAllCalls)
	}
}

func TestCreate_EmptyImageRootfs(t *testing.T) {
	mockFs := newMockFsWriter()
	store := newContainerStoreWith("/containers", mockFs)

	_, _, err := store.Create("")
	if err == nil {
		t.Fatal("expected error for empty imageRootfs, got nil")
	}
	if !errors.Is(err, ErrInvalidImageRootfs) {
		t.Errorf("expected ErrInvalidImageRootfs, got: %v", err)
	}
}

func TestCreate_MkdirAllError_CleansUp(t *testing.T) {
	mockFs := newMockFsWriter()
	mockFs.mkdirAllErr = errors.New("disk full")
	store := newContainerStoreWith("/containers", mockFs)

	_, _, err := store.Create("/images/alpine/rootfs")
	if err == nil {
		t.Fatal("expected error on MkdirAll failure, got nil")
	}

	if len(mockFs.removeCalls) == 0 {
		t.Error("expected cleanup RemoveAll call on error")
	}
}

func TestMergedPath_ReturnsCorrectPath(t *testing.T) {
	mockFs := newMockFsWriter()
	baseDir := "/var/lib/minicontainer/containers"
	store := newContainerStoreWith(baseDir, mockFs)

	id := "a1b2c3d4e5f6"
	expected := filepath.Join(baseDir, id, mergedDirName)
	if path := store.MergedPath(id); path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestUpperAndWorkPath(t *testing.T) {
	mockFs := newMockFsWriter()
	baseDir := "/containers"
	store := newContainerStoreWith(baseDir, mockFs)

	id := "c12345678901"
	expectedUpper := filepath.Join(baseDir, id, upperDirName)
	expectedWork := filepath.Join(baseDir, id, workDirName)

	if store.UpperPath(id) != expectedUpper {
		t.Errorf("expected upper %q, got %q", expectedUpper, store.UpperPath(id))
	}
	if store.WorkPath(id) != expectedWork {
		t.Errorf("expected work %q, got %q", expectedWork, store.WorkPath(id))
	}
}

func TestDestroy_RemovesAllDirs(t *testing.T) {
	mockFs := newMockFsWriter()
	baseDir := "/containers"
	store := newContainerStoreWith(baseDir, mockFs)

	id, _, err := store.Create("/images/alpine/rootfs")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := store.Destroy(id); err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}

	expectedContainerDir := filepath.Join(baseDir, id)
	found := false
	for _, call := range mockFs.removeCalls {
		if call == expectedContainerDir {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected %q to be removed in %v", expectedContainerDir, mockFs.removeCalls)
	}

	// Destroying again should return ErrContainerNotFound
	if err := store.Destroy(id); !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound on double destroy, got: %v", err)
	}
}

func TestDestroy_ErrorOnMissingID(t *testing.T) {
	mockFs := newMockFsWriter()
	store := newContainerStoreWith("/containers", mockFs)

	err := store.Destroy("nonexistent01")
	if err == nil {
		t.Fatal("expected error on missing container ID, got nil")
	}
	if !errors.Is(err, ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound, got: %v", err)
	}
}

func TestDestroy_InvalidID(t *testing.T) {
	mockFs := newMockFsWriter()
	store := newContainerStoreWith("/containers", mockFs)

	invalidIDs := []string{"", "  ", "../escape", "a/b", "a\\b"}
	for _, id := range invalidIDs {
		err := store.Destroy(id)
		if err == nil {
			t.Errorf("Destroy(%q): expected error, got nil", id)
		}
		if !errors.Is(err, ErrInvalidContainerID) {
			t.Errorf("Destroy(%q): expected ErrInvalidContainerID, got: %v", id, err)
		}
	}
}

func TestNewDefaultContainerStore(t *testing.T) {
	store := NewDefaultContainerStore()
	if store == nil {
		t.Fatal("expected non-nil ContainerStore from NewDefaultContainerStore()")
	}
}

func TestGenerateContainerID(t *testing.T) {
	id, err := generateContainerID()
	if err != nil {
		t.Fatalf("generateContainerID failed: %v", err)
	}
	if len(id) != 12 {
		t.Errorf("expected 12 chars, got %d (%q)", len(id), id)
	}
	// Verify hex
	for _, r := range id {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("invalid hex character %q in ID %q", r, id)
		}
	}
}
