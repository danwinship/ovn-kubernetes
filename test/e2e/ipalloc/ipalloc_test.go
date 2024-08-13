package ipalloc

import (
	"fmt"
	"net"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestUtilSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "node ip alloc suite")
}

func TestAllocateNext(t *testing.T) {
	tests := []struct {
		desc   string
		input  *net.IPNet
		output []net.IP
	}{
		{
			desc:   "allocates IPs sequentially",
			input:  mustParseCIDRIncIP("192.168.1.5/24"),
			output: []net.IP{net.ParseIP("192.168.1.6"), net.ParseIP("192.168.1.7"), net.ParseIP("192.168.1.8")},
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%d:%s", i, tc.desc), func(t *testing.T) {
			nodeIPAlloc := newIPAllocator(tc.input)
			for _, expectedIP := range tc.output {
				allocatedIP, err := nodeIPAlloc.AllocateNextIP()
				if err != nil {
					t.Errorf("failed to allocated next IP: %v", err)
				}
				if !allocatedIP.Equal(expectedIP) {
					t.Errorf("Expected IP %q, but got %q", expectedIP.String(), allocatedIP.String())
				}
			}
		})
	}
}

// mustParseCIDRIncIP parses the IP and CIDR. It adds the IP to the returned IPNet.
func mustParseCIDRIncIP(cidr string) *net.IPNet {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CIDR %q: %v", cidr, err))
	}
	ipNet.IP = ip
	return ipNet
}
