package ipalloc

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// primaryIPAllocator attempts to allocate an IP in the same subnet as a nodes primary network
type primaryIPAllocator struct {
	mu         *sync.Mutex
	v4         ipAllocator
	v6         ipAllocator
	nodeClient v1.NodeInterface
}

var pia primaryIPAllocator

func InitPrimaryIPAllocator(nodeClient v1.NodeInterface) error {
	var err error
	pia, err = newPrimaryIPAllocator(nodeClient)
	return err
}

func NewPrimaryIPv4() (net.IP, error) {
	return pia.AllocateNextV4()
}

func NewPrimaryIPv6() (net.IP, error) {
	return pia.AllocateNextV6()
}

func newPrimaryIPAllocator(nodeClient v1.NodeInterface) (primaryIPAllocator, error) {
	ipa := primaryIPAllocator{mu: &sync.Mutex{}, nodeClient: nodeClient}

	nodes, err := nodeClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return ipa, fmt.Errorf("failed to get a list of node(s): %v", err)
	}
	if len(nodes.Items) == 0 {
		return ipa, fmt.Errorf("expected at least one node but found zero")
	}
	nodePrimaryIPs, err := util.ParseNodePrimaryIfAddr(&nodes.Items[0])
	if err != nil {
		return ipa, fmt.Errorf("failed to parse node primary interface address from Node object: %v", err)
	}
	if nodePrimaryIPs.V4.Net != nil {
		ipa.v4 = newIPAllocator(nodePrimaryIPs.V4.Net)
	}
	if nodePrimaryIPs.V6.Net != nil {
		ipa.v6 = newIPAllocator(nodePrimaryIPs.V6.Net)
	}
	return ipa, nil
}

func (pia primaryIPAllocator) AllocateNextV4() (net.IP, error) {
	if pia.v4.net == nil {
		return nil, fmt.Errorf("ipv4 network is not set")
	}
	pia.mu.Lock()
	defer pia.mu.Unlock()
	return allocateIP(pia.nodeClient, pia.v4.AllocateNextIP)
}

func (pia primaryIPAllocator) AllocateNextV6() (net.IP, error) {
	if pia.v6.net == nil {
		return nil, fmt.Errorf("ipv6 network is not set")
	}
	pia.mu.Lock()
	defer pia.mu.Unlock()
	return allocateIP(pia.nodeClient, pia.v6.AllocateNextIP)
}

type allocNextFn func() (net.IP, error)

func allocateIP(nodeClient v1.NodeInterface, allocateFn allocNextFn) (net.IP, error) {
	nodeList, err := nodeClient.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %v", err)
	}
	for {
		nextIP, err := allocateFn()
		if err != nil {
			return nil, fmt.Errorf("failed to allocated next IP address: %v", err)
		}
		firstOctet := nextIP[len(nextIP)-1]
		// skip 0 and 1
		if firstOctet == 0 || firstOctet == 1 {
			continue
		}
		isConflict, err := isConflictWithExistingHostIPs(nodeList.Items, nextIP)
		if err != nil {
			return nil, fmt.Errorf("failed to determine if IP conflicts with existing IPs: %v", err)
		}
		if !isConflict {
			return nextIP, nil
		}
	}
}

func isConflictWithExistingHostIPs(nodes []corev1.Node, ip net.IP) (bool, error) {
	ipStr := ip.String()
	for _, node := range nodes {
		nodeIPsSet, err := util.ParseNodeHostCIDRsDropNetMask(&node)
		if err != nil {
			return false, fmt.Errorf("failed to parse node %s primary annotation info: %v", node.Name, err)
		}
		if nodeIPsSet.Has(ipStr) {
			return true, nil
		}
	}
	return false, nil
}
