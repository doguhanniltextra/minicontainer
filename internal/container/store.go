package container

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// DefaultContainerBase is the default base path where per-container directories are stored.
	DefaultContainerBase = "/var/lib/minicontainer/containers"

	upperDirName  = "upper"
	workDirName   = "work"
	mergedDirName = "merged"
)

var (
	// ErrContainerNotFound is returned when attempting to operate on a non-existent container ID.
	ErrContainerNotFound = errors.New("container not found")
	// ErrInvalidContainerID is returned when a container ID is malformed or invalid.
	ErrInvalidContainerID = errors.New("invalid container id")
	// ErrInvalidImageRootfs is returned when an empty or invalid image rootfs is passed.
	ErrInvalidImageRootfs = errors.New("invalid image rootfs")
)

// ContainerStore manages per-container OverlayFS directory lifecycle.
type ContainerStore interface {
	// Create allocates upper/work/merged dirs for a new container, returns container ID.
	Create(imageRootfs string) (id string, merged string, err error)
	// Destroy removes all dirs associated with the given container ID.
	Destroy(id string) error
	// MergedPath returns the merged directory path for an existing container.
	MergedPath(id string) string
	// UpperPath returns the upper directory path for an existing container.
	UpperPath(id string) string
	// WorkPath returns the work directory path for an existing container.
	WorkPath(id string) string
}

// localContainerStore implements ContainerStore backed by a local filesystem directory.
type localContainerStore struct {
	baseDir    string
	fs         fsWriter
	mu         sync.Mutex
	containers map[string]bool
}

// NewDefaultContainerStore creates a ContainerStore using DefaultContainerBase and realFsWriter.
func NewDefaultContainerStore() ContainerStore {
	return NewContainerStore(DefaultContainerBase)
}

// NewContainerStore creates a ContainerStore at the given base directory.
func NewContainerStore(baseDir string) ContainerStore {
	return newContainerStoreWith(baseDir, realFsWriter{})
}

// newContainerStoreWith creates a localContainerStore with an injected fsWriter.
func newContainerStoreWith(baseDir string, fs fsWriter) *localContainerStore {
	return &localContainerStore{
		baseDir:    baseDir,
		fs:         fs,
		containers: make(map[string]bool),
	}
}

// generateContainerID produces a collision-resistant 12-char hex ID.
func generateContainerID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating container id: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}

// Create allocates upper/work/merged dirs for a new container and returns container ID and merged path.
func (s *localContainerStore) Create(imageRootfs string) (string, string, error) {
	if strings.TrimSpace(imageRootfs) == "" {
		return "", "", ErrInvalidImageRootfs
	}

	id, err := generateContainerID()
	if err != nil {
		return "", "", err
	}

	containerDir := filepath.Join(s.baseDir, id)
	upper := filepath.Join(containerDir, upperDirName)
	work := filepath.Join(containerDir, workDirName)
	merged := filepath.Join(containerDir, mergedDirName)

	for _, dir := range []string{upper, work, merged} {
		if err := s.fs.MkdirAll(dir, 0755); err != nil {
			_ = s.fs.RemoveAll(containerDir)
			return "", "", fmt.Errorf("creating container directory %s: %w", dir, err)
		}
	}

	s.mu.Lock()
	s.containers[id] = true
	s.mu.Unlock()

	return id, merged, nil
}

// Destroy removes all dirs associated with the given container ID.
func (s *localContainerStore) Destroy(id string) error {
	if err := validateContainerID(id); err != nil {
		return err
	}

	s.mu.Lock()
	if !s.containers[id] {
		s.mu.Unlock()
		return fmt.Errorf("%w: container %q", ErrContainerNotFound, id)
	}
	delete(s.containers, id)
	s.mu.Unlock()

	containerDir := filepath.Join(s.baseDir, id)
	if err := s.fs.RemoveAll(containerDir); err != nil {
		return fmt.Errorf("removing container directory %s: %w", containerDir, err)
	}

	return nil
}

// MergedPath returns the merged directory path for an existing container.
func (s *localContainerStore) MergedPath(id string) string {
	return filepath.Join(s.baseDir, id, mergedDirName)
}

// UpperPath returns the upper directory path for an existing container.
func (s *localContainerStore) UpperPath(id string) string {
	return filepath.Join(s.baseDir, id, upperDirName)
}

// WorkPath returns the work directory path for an existing container.
func (s *localContainerStore) WorkPath(id string) string {
	return filepath.Join(s.baseDir, id, workDirName)
}

func validateContainerID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id cannot be empty", ErrInvalidContainerID)
	}
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("%w: id contains invalid characters", ErrInvalidContainerID)
	}
	return nil
}
