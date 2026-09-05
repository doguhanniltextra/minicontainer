package container

import (
	"errors"
	"sync"
	"testing"
)

func TestNewIPAM_ValidSubnetAndGateway(t *testing.T) {
	ipam, err := NewIPAM(DefaultSubnetCIDR, DefaultGatewayIP)
	if err != nil {
		t.Fatalf("expected NewIPAM to succeed, got error: %v", err)
	}
	if ipam == nil {
		t.Fatal("expected non-nil IPAM instance")
	}
}

func TestNewIPAM_InvalidSubnet(t *testing.T) {
	_, err := NewIPAM("invalid-cidr", "172.19.0.1")
	if err == nil {
		t.Error("expected error for invalid subnet CIDR, got nil")
	}
}

func TestNewIPAM_InvalidGateway(t *testing.T) {
	_, err := NewIPAM("172.19.0.0/16", "999.999.999.999")
	if err == nil {
		t.Error("expected error for invalid gateway IP, got nil")
	}
}

func TestNewIPAM_GatewayNotInSubnet(t *testing.T) {
	_, err := NewIPAM("172.19.0.0/16", "10.0.0.1")
	if err == nil {
		t.Error("expected error when gateway IP is outside subnet, got nil")
	}
}

func TestAllocate_StartsAtFirstUsableIP(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	ip, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	expected := "172.19.0.2/16"
	if ip != expected {
		t.Errorf("expected first allocated IP to be %q, got %q", expected, ip)
	}
}

func TestAllocate_Sequential(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	ip1, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate 1 failed: %v", err)
	}
	ip2, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate 2 failed: %v", err)
	}
	ip3, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate 3 failed: %v", err)
	}

	if ip1 != "172.19.0.2/16" {
		t.Errorf("expected ip1 to be 172.19.0.2/16, got %s", ip1)
	}
	if ip2 != "172.19.0.3/16" {
		t.Errorf("expected ip2 to be 172.19.0.3/16, got %s", ip2)
	}
	if ip3 != "172.19.0.4/16" {
		t.Errorf("expected ip3 to be 172.19.0.4/16, got %s", ip3)
	}
}

func TestRelease_ReusesIP(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	ip1, _ := ipam.Allocate() // 172.19.0.2/16
	ip2, _ := ipam.Allocate() // 172.19.0.3/16

	if err := ipam.Release(ip1); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Next allocation should reuse ip1
	ipReused, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate after release failed: %v", err)
	}

	if ipReused != ip1 {
		t.Errorf("expected reused IP to be %q, got %q (ip2 was %q)", ip1, ipReused, ip2)
	}
}

func TestRelease_AcceptsPlainIPWithoutCIDR(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	ip1, _ := ipam.Allocate() // 172.19.0.2/16
	// Releasing "172.19.0.2" without "/16" suffix should also work
	if err := ipam.Release("172.19.0.2"); err != nil {
		t.Fatalf("expected plain IP release to succeed, got: %v", err)
	}

	ipReused, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("Allocate after release failed: %v", err)
	}
	if ipReused != ip1 {
		t.Errorf("expected %q to be reused, got %q", ip1, ipReused)
	}
}

func TestRelease_UnallocatedError(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	err = ipam.Release("172.19.0.50/16")
	if !errors.Is(err, ErrIPNotAllocated) {
		t.Errorf("expected ErrIPNotAllocated, got %v", err)
	}
}

func TestRelease_InvalidIP(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	err = ipam.Release("not-an-ip")
	if !errors.Is(err, ErrInvalidIP) {
		t.Errorf("expected ErrInvalidIP, got %v", err)
	}

	err = ipam.Release("10.0.0.1/16")
	if !errors.Is(err, ErrInvalidIP) {
		t.Errorf("expected ErrInvalidIP for IP outside subnet, got %v", err)
	}
}

func TestRelease_GatewayCannotBeReleased(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	err = ipam.Release("172.19.0.1/16")
	if err == nil {
		t.Error("expected error when attempting to release gateway IP, got nil")
	}
}

func TestAllocate_SubnetExhaustion(t *testing.T) {
	// A /30 network has 4 addresses:
	// .0 (network), .1 (gateway), .2 (usable host), .3 (broadcast)
	// So only 1 container IP can be allocated!
	ipam, err := NewIPAM("172.19.0.0/30", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	ip1, err := ipam.Allocate()
	if err != nil {
		t.Fatalf("expected first allocation to succeed, got %v", err)
	}
	if ip1 != "172.19.0.2/30" {
		t.Errorf("expected 172.19.0.2/30, got %s", ip1)
	}

	// Next allocation must exhaust the pool
	_, err = ipam.Allocate()
	if !errors.Is(err, ErrSubnetExhausted) {
		t.Errorf("expected ErrSubnetExhausted, got %v", err)
	}
}

func TestAllocate_ConcurrentSafe(t *testing.T) {
	ipam, err := NewIPAM("172.19.0.0/16", "172.19.0.1")
	if err != nil {
		t.Fatalf("NewIPAM failed: %v", err)
	}

	const count = 50
	allocated := make(chan string, count)
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip, err := ipam.Allocate()
			if err != nil {
				t.Errorf("concurrent Allocate failed: %v", err)
				return
			}
			allocated <- ip
		}()
	}

	wg.Wait()
	close(allocated)

	seen := make(map[string]bool)
	for ip := range allocated {
		if seen[ip] {
			t.Errorf("duplicate IP allocated concurrently: %s", ip)
		}
		seen[ip] = true
	}

	if len(seen) != count {
		t.Errorf("expected %d unique IPs, got %d", count, len(seen))
	}
}
