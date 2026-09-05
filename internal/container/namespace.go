package container

import "syscall"

const (
	// DefaultHostUID is the default host UID to map container root (UID 0) to.
	// 65534 is the standard Linux "nobody" user.
	DefaultHostUID = 65534

	// DefaultHostGID is the default host GID to map container root (GID 0) to.
	// 65534 is the standard Linux "nogroup" group.
	DefaultHostGID = 65534
)

// buildSysProcAttr configures kernel-level attributes for the child process.
// By default, it requests creation of 5 namespaces (PID, UTS, NET, MNT, USER)
// with container UID/GID 0 mapped to host nobody/nogroup (65534).
func buildSysProcAttr() *syscall.SysProcAttr {
	return buildSysProcAttrFor(Config{
		UserNamespace: true,
		HostUID:       DefaultHostUID,
		HostGID:       DefaultHostGID,
	})
}

// buildSysProcAttrFor configures kernel-level attributes based on the provided Config.
// When UserNamespace is enabled, it adds CLONE_NEWUSER and maps container UID/GID 0
// to HostUID/HostGID (defaulting to nobody/nogroup).
func buildSysProcAttrFor(cfg Config) *syscall.SysProcAttr {
	flags := uintptr(syscall.CLONE_NEWPID |
		syscall.CLONE_NEWUTS |
		syscall.CLONE_NEWNET |
		syscall.CLONE_NEWNS)

	var uidMaps []syscall.SysProcIDMap
	var gidMaps []syscall.SysProcIDMap

	if cfg.UserNamespace {
		flags |= syscall.CLONE_NEWUSER

		hostUID := cfg.HostUID
		if hostUID == 0 {
			hostUID = DefaultHostUID
		}
		hostGID := cfg.HostGID
		if hostGID == 0 {
			hostGID = DefaultHostGID
		}

		uidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: hostUID, Size: 1},
		}
		gidMaps = []syscall.SysProcIDMap{
			{ContainerID: 0, HostID: hostGID, Size: 1},
		}
	}

	return &syscall.SysProcAttr{
		Cloneflags:  flags,
		UidMappings: uidMaps,
		GidMappings: gidMaps,
		Setsid:      true,
		Setctty:     true,
		Ctty:        0,
	}
}

// setHostname changes the hostname inside the current UTS namespace.
// Accepts a hostnamer to allow mocking in unit tests.
// Note: Must be called from within the child process after entering the new UTS namespace.
func setHostname(h hostnamer, hostname string) error {
	return h.Sethostname([]byte(hostname))
}
