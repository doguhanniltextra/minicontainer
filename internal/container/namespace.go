package container

import "syscall"

// buildSysProcAttr configures kernel-level attributes for the child process.
// It requests creation of 4 new namespaces (PID, UTS, NET, MNT).
func buildSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | // Isolated Process ID counter (PID 1 inside)
			syscall.CLONE_NEWUTS | // Isolated Hostname/Domainname
			syscall.CLONE_NEWNET | // Isolated Network stack (only loopback initially)
			syscall.CLONE_NEWNS, // Isolated Mount table
		Setsid:  true, // Create a new session so the child can own the terminal
		Setctty: true, // Assign the controlling terminal to the new session
		Ctty:    0,    // fd 0 (stdin) becomes the controlling terminal
	}
}

// setHostname changes the hostname inside the current UTS namespace.
// Accepts a hostnamer to allow mocking in unit tests.
// Note: Must be called from within the child process after entering the new UTS namespace.
func setHostname(h hostnamer, hostname string) error {
	return h.Sethostname([]byte(hostname))
}
