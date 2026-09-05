package container

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
)

const (
	// DefaultSubnetCIDR is the private subnet allocated to minicontainer.
	DefaultSubnetCIDR = "172.19.0.0/16"

	// DefaultGatewayIP is the IP reserved for the host bridge (mc-br0).
	DefaultGatewayIP = "172.19.0.1"
)

var (
	// ErrSubnetExhausted is returned when all usable IP addresses in the subnet are allocated.
	ErrSubnetExhausted = errors.New("subnet IP address pool is exhausted")

	// ErrInvalidIP is returned when an IP address cannot be parsed or is outside the subnet.
	ErrInvalidIP = errors.New("invalid IP address")

	// ErrIPNotAllocated is returned when attempting to release an IP that is not currently allocated.
	ErrIPNotAllocated = errors.New("IP address was not allocated")
)

// IPAM manages in-memory IP address allocation for a container subnet.
type IPAM struct {
	mu          sync.Mutex
	subnet      *net.IPNet
	gatewayUint uint32
	netStart    uint32
	netEnd      uint32
	allocated   map[uint32]bool
}

// NewIPAM creates a new IP address manager for the given subnet CIDR and gateway.
// It reserves the network address, broadcast address, and gateway IP.
func NewIPAM(subnetCIDR, gateway string) (*IPAM, error) {
	_, ipnet, err := net.ParseCIDR(subnetCIDR)
	if err != nil {
		return nil, fmt.Errorf("parsing subnet CIDR %q: %w", subnetCIDR, err)
	}

	ip4 := ipnet.IP.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("subnet %q is not IPv4: %w", subnetCIDR, ErrInvalidIP)
	}

	gw := net.ParseIP(gateway)
	if gw == nil {
		return nil, fmt.Errorf("gateway %q: %w", gateway, ErrInvalidIP)
	}
	gw4 := gw.To4()
	if gw4 == nil {
		return nil, fmt.Errorf("gateway %q is not IPv4: %w", gateway, ErrInvalidIP)
	}

	if !ipnet.Contains(gw4) {
		return nil, fmt.Errorf("gateway %s is outside subnet %s", gateway, subnetCIDR)
	}

	mask := binary.BigEndian.Uint32(ipnet.Mask)
	netStart := ipToUint32(ip4) & mask
	netEnd := netStart | ^mask
	gatewayUint := ipToUint32(gw4)

	allocated := make(map[uint32]bool)
	// Reserve network address, broadcast address, and gateway
	allocated[netStart] = true
	allocated[netEnd] = true
	allocated[gatewayUint] = true

	return &IPAM{
		subnet:      ipnet,
		gatewayUint: gatewayUint,
		netStart:    netStart,
		netEnd:      netEnd,
		allocated:   allocated,
	}, nil
}

// Allocate returns the next available IP address with its CIDR mask (e.g. "172.19.0.2/16").
func (ipam *IPAM) Allocate() (string, error) {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	for cur := ipam.netStart + 1; cur < ipam.netEnd; cur++ {
		if !ipam.allocated[cur] {
			ipam.allocated[cur] = true
			ones, _ := ipam.subnet.Mask.Size()
			return fmt.Sprintf("%s/%d", uint32ToIP(cur).String(), ones), nil
		}
	}

	return "", ErrSubnetExhausted
}

// Release marks an allocated IP address as free.
// Accepts either a CIDR string (e.g. "172.19.0.2/16") or a bare IP string ("172.19.0.2").
func (ipam *IPAM) Release(ipCIDR string) error {
	ipam.mu.Lock()
	defer ipam.mu.Unlock()

	var ip net.IP
	if strings.Contains(ipCIDR, "/") {
		parsedIP, _, err := net.ParseCIDR(ipCIDR)
		if err != nil {
			return ErrInvalidIP
		}
		ip = parsedIP
	} else {
		ip = net.ParseIP(ipCIDR)
		if ip == nil {
			return ErrInvalidIP
		}
	}

	ip4 := ip.To4()
	if ip4 == nil || !ipam.subnet.Contains(ip4) {
		return ErrInvalidIP
	}

	val := ipToUint32(ip4)
	if val == ipam.gatewayUint {
		return errors.New("cannot release gateway IP")
	}

	if !ipam.allocated[val] {
		return ErrIPNotAllocated
	}

	delete(ipam.allocated, val)
	return nil
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return binary.BigEndian.Uint32(ip)
}

func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}
