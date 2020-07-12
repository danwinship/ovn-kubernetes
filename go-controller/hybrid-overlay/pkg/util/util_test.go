package util

import (
	"net"
	"testing"

	"github.com/ovn-org/ovn-kubernetes/go-controller/hybrid-overlay/pkg/types"
	ovntest "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/testing"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"

	kapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseHybridOverlaySubnets(t *testing.T) {
	type testcase struct {
		name       string
		annotation string
		subnets    []*net.IPNet
		expectErr  bool
	}

	testcases := []testcase{
		{
			name:       "single-stack",
			annotation: "1.2.3.0/24",
			subnets:    ovntest.MustParseIPNets("1.2.3.0/24"),
		},
		{
			name:       "dual-stack",
			annotation: `["1.2.3.0/24","fd95::/64"]`,
			subnets:    ovntest.MustParseIPNets("1.2.3.0/24", "fd95::/64"),
		},
		{
			name:       "unset",
			subnets:    nil,
			expectErr:  false,
		},
		{
			name:       "bad single-stack",
			annotation: "blah",
			subnets:    nil,
			expectErr:  true,
		},
		{
			name:       "bad dual-stack",
			annotation: `["1.2.3.0/24", "blah"]`,
			subnets:    nil,
			expectErr:  true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			node := &kapi.Node{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}
			if tc.annotation != "" {
				node.Annotations[types.HybridOverlayNodeSubnet] = tc.annotation
			}
			subnets, err := ParseHybridOverlayHostSubnets(node)
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("expected success, got %v", err)
				}
			} else if tc.expectErr {
				t.Fatalf("expected error, got %v", subnets)
			}
			if len(subnets) != len(tc.subnets) {
				t.Fatalf("expected %#v, got %#v", util.JoinIPNets(tc.subnets, ","), util.JoinIPNets(subnets, ","))
			}
			for i := range subnets {
				if subnets[i].String() != tc.subnets[i].String() {
					t.Fatalf("expected %#v, got %#v", util.JoinIPNets(tc.subnets, ","), util.JoinIPNets(subnets, ","))
				}
			}
		})
	}
}
