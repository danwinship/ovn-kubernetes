//go:build linux
// +build linux

package node

import (
	"net"

	utilnet "k8s.io/utils/net"

	"github.com/coreos/go-iptables/iptables"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/config"
	nodeipt "github.com/ovn-org/ovn-kubernetes/go-controller/pkg/node/iptables"
	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"
)


const (
	// Legacy iptables service chains
	iptableNodePortChain   = "OVN-KUBE-NODEPORT"
	iptableExternalIPChain = "OVN-KUBE-EXTERNALIP"
	iptableETPChain        = "OVN-KUBE-ETP"
	iptableITPChain        = "OVN-KUBE-ITP"
)

// getIPTablesProtocol returns the IPTables protocol matching the protocol (v4/v6) of provided IP string
func getIPTablesProtocol(ip string) iptables.Protocol {
	if utilnet.IsIPv6String(ip) {
		return iptables.ProtocolIPv6
	}
	return iptables.ProtocolIPv4
}

func getGatewayInitRules(chain string, proto iptables.Protocol) []nodeipt.Rule {
	iptRules := []nodeipt.Rule{}
	if chain == iptableITPChain {
		iptRules = append(iptRules,
			nodeipt.Rule{
				Table:    "mangle",
				Chain:    "OUTPUT",
				Args:     []string{"-j", chain},
				Protocol: proto,
			},
		)
	} else {
		iptRules = append(iptRules,
			nodeipt.Rule{
				Table:    "nat",
				Chain:    "PREROUTING",
				Args:     []string{"-j", chain},
				Protocol: proto,
			},
		)
	}
	if chain != iptableETPChain { // ETP chain only meant for external traffic
		iptRules = append(iptRules,
			nodeipt.Rule{
				Table:    "nat",
				Chain:    "OUTPUT",
				Args:     []string{"-j", chain},
				Protocol: proto,
			},
		)
	}
	return iptRules
}

func getGatewayForwardRules(cidrs []*net.IPNet) []nodeipt.Rule {
	var returnRules []nodeipt.Rule
	protocols := make(map[iptables.Protocol]struct{})

	// Add rules for all CIDRs.
	for _, cidr := range cidrs {
		protocol := getIPTablesProtocol(cidr.IP.String())
		protocols[protocol] = struct{}{}

		returnRules = append(returnRules, []nodeipt.Rule{
			{
				Table: "filter",
				Chain: "FORWARD",
				Args: []string{
					"-s", cidr.String(),
					"-j", "ACCEPT",
				},
				Protocol: protocol,
			},
			{
				Table: "filter",
				Chain: "FORWARD",
				Args: []string{
					"-d", cidr.String(),
					"-j", "ACCEPT",
				},
				Protocol: protocol,
			},
		}...)
	}

	// Add rules for MasqueraIPs.
	for protocol := range protocols {
		masqueradeIP := config.Gateway.MasqueradeIPs.V4OVNMasqueradeIP
		if protocol == iptables.ProtocolIPv6 {
			masqueradeIP = config.Gateway.MasqueradeIPs.V6OVNMasqueradeIP
		}
		returnRules = append(returnRules, getMasqueradeIpTablesForwardRules(masqueradeIP, protocol)...)
	}

	return returnRules
}

// cleanupStaleMasqueradeIptablesRules deletes all iptables rules may have been added for a given masquerade IP.
// This is only called to clean up legacy state, so it is best-effort and ignores errors.
func cleanupStaleMasqueradeIptablesRules(masqueradeIP net.IP) {
	_ = nodeipt.DelRules(getMasqueradeIpTablesForwardRules(masqueradeIP, getIPTablesProtocol(masqueradeIP.String())))
	_ = nodeipt.DelRules(getMasqueradeIpTablesNATRules(masqueradeIP, getIPTablesProtocol(masqueradeIP.String())))
}

func getMasqueradeIpTablesForwardRules(masqueradeIP net.IP, protocol iptables.Protocol) []nodeipt.Rule {
	return []nodeipt.Rule{
		{
			Table: "filter",
			Chain: "FORWARD",
			Args: []string{
				"-s", masqueradeIP.String(),
				"-j", "ACCEPT",
			},
			Protocol: protocol,
		},
		{
			Table: "filter",
			Chain: "FORWARD",
			Args: []string{
				"-d", masqueradeIP.String(),
				"-j", "ACCEPT",
			},
			Protocol: protocol,
		},
	}
}

func getMasqueradeIpTablesNATRules(masqueradeIP net.IP, protocol iptables.Protocol) []nodeipt.Rule {
	return []nodeipt.Rule{
		{
			Table: "nat",
			Chain: "POSTROUTING",
			Args: []string{
				"-s", masqueradeIP.String(),
				"-j", "MASQUERADE",
			},
			Protocol: protocol,
		},
	}
}

// cleanupExternalBridgeServiceIPTForwardingRules removes iptables rules which might
// have been added to disable forwarding. This is only called to clean up legacy state,
// so it is best-effort and ignores errors.
func cleanupExternalBridgeServiceIPTForwardingRules(cidrs []*net.IPNet) {
	_ = nodeipt.DelRules(getGatewayForwardRules(cidrs))
}

func getLocalGatewayFilterRules(ifname string, cidr *net.IPNet) []nodeipt.Rule {
	// Allow packets to/from the gateway interface in case defaults deny
	protocol := getIPTablesProtocol(cidr.IP.String())
	return []nodeipt.Rule{
		{
			Table: "filter",
			Chain: "FORWARD",
			Args: []string{
				"-o", ifname,
				"-j", "ACCEPT",
			},
			Protocol: protocol,
		},
		{
			Table: "filter",
			Chain: "FORWARD",
			Args: []string{
				"-i", ifname,
				"-j", "ACCEPT",
			},
			Protocol: protocol,
		},
	}
}

// cleanupLocalGatewayIPTFilterRules removes iptables rules for interfaces.
// This is only called to clean up legacy state, so it is best-effort and ignores errors.
func cleanupLocalGatewayIPTFilterRules(ifname string, cidr *net.IPNet) {
	_ = nodeipt.DelRules(getLocalGatewayFilterRules(ifname, cidr))
}

// cleanupGatewayNodePortIPTables removes iptables rules related to Services.
// This is only called to clean up legacy state, so it is best-effort and ignores errors.
func cleanupGatewayNodePortIPTables() {
	// We clean up both IPv4 and IPv6, regardless of what is currently in use
	for _, proto := range []iptables.Protocol{iptables.ProtocolIPv4, iptables.ProtocolIPv6} {
		ipt, err := util.GetIPTablesHelper(proto)
		if err != nil {
			continue
		}
		for _, chain := range []string{iptableITPChain, iptableNodePortChain, iptableExternalIPChain, iptableETPChain} {
			_ = nodeipt.DelRules(getGatewayInitRules(chain, proto))
			_ = ipt.ClearChain("nat", chain)
			_ = ipt.DeleteChain("nat", chain)
			if chain == iptableITPChain {
				_ = ipt.ClearChain("mangle", chain)
				_ = ipt.DeleteChain("mangle", chain)
			}
		}
	}
}
