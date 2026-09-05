package image

import (
	"errors"
)

// Sentinel errors returned by Store implementations.
var (
	ErrImageNotFound = errors.New("image not found")
	ErrImageExists   = errors.New("image already exists")
	ErrInvalidName   = errors.New("invalid image name")
)

const (
	// DefaultImageBase is the default base path where readonly images are stored.
	DefaultImageBase = "/var/lib/minicontainer/images"

	rootfsDirName    = "rootfs"
	metadataFileName = "metadata.json"
)

// Metadata represents basic metadata recorded when an image is imported.
type Metadata struct {
	Name    string `json:"name"`
	Created string `json:"created"`
}

// Store manages readonly base images on disk.
type Store interface {
	// List returns all available image names.
	List() ([]string, error)
	// Get returns the rootfs path for a named image.
	Get(name string) (rootfsPath string, err error)
	// Import copies a directory into the image store as a new image.
	Import(name, srcPath string) error
	// Remove deletes an image from the store.
	Remove(name string) error
}

// New creates a new Store backed by the local filesystem at the given base directory.
func New(baseDir string) Store {
	return newLocalStore(baseDir)
}

// NewDefault creates a new Store using DefaultImageBase.
func NewDefault() Store {
	return New(DefaultImageBase)
}
