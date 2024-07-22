package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/ovn-org/ovn-kubernetes/test/e2e/deployment"
	"github.com/ovn-org/ovn-kubernetes/test/e2e/provider"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	bridgeName = "ovsbr1"
	add        = "add-br"
	del        = "del-br"
)

func setupUnderlay(ovsPods []v1.Pod, portName string, nadConfig networkAttachmentConfig) error {
	for _, ovsPod := range ovsPods {
		if err := addOVSBridge(ovsPod.Name, bridgeName); err != nil {
			return err
		}

		if nadConfig.vlanID > 0 {
			if err := ovsEnableVLANAccessPort(ovsPod.Name, bridgeName, portName, nadConfig.vlanID); err != nil {
				return err
			}
		} else {
			if err := ovsAttachPortToBridge(ovsPod.Name, bridgeName, portName); err != nil {
				return err
			}
		}

		if err := configureBridgeMappings(
			ovsPod.Name,
			defaultNetworkBridgeMapping(),
			bridgeMapping(nadConfig.networkName, bridgeName),
		); err != nil {
			return err
		}
	}
	return nil
}

func teardownUnderlay(ovsPods []v1.Pod) error {
	for _, ovsPod := range ovsPods {
		if err := removeOVSBridge(ovsPod.Name, bridgeName); err != nil {
			return err
		}
	}
	return nil
}

func ovsPods(clientSet clientset.Interface) []v1.Pod {
	const (
		ovsNodeLabel = "app=ovs-node"
	)
	pods, err := clientSet.CoreV1().Pods(deployment.Get().OVNKubernetesNamespace()).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: ovsNodeLabel},
	)
	if err != nil {
		return nil
	}
	return pods.Items
}

func addOVSBridge(ovnNodeName string, bridgeName string) error {
	_, err := provider.Get().ExecK8NodeCommand(ovnNodeName, []string{"ovs-vsctl", add, bridgeName})
	if err != nil {
		return fmt.Errorf("failed to ADD OVS bridge %s: %v", bridgeName, err)
	}
	return nil
}

func removeOVSBridge(ovnNodeName string, bridgeName string) error {
	_, err := provider.Get().ExecK8NodeCommand(ovnNodeName, []string{"ovs-vsctl", del, bridgeName})
	if err != nil {
		return fmt.Errorf("failed to DELETE OVS bridge %s: %v", bridgeName, err)
	}
	return nil
}

func ovsAttachPortToBridge(ovsNodeName string, bridgeName string, portName string) error {
	_, err := provider.Get().ExecK8NodeCommand(ovsNodeName, []string{"ovs-vsctl", "add-port", bridgeName, portName})
	if err != nil {
		return fmt.Errorf("failed to add port %s to OVS bridge %s: %v", portName, bridgeName, err)
	}

	return nil
}

func ovsEnableVLANAccessPort(ovsNodeName string, bridgeName string, portName string, vlanID int) error {
	_, err := provider.Get().ExecK8NodeCommand(ovsNodeName, []string{"ovs-vsctl", "add-port", bridgeName, portName, fmt.Sprintf("tag=%d", vlanID), "vlan_mode=access"})
	if err != nil {
		return fmt.Errorf("failed to add port %s to OVS bridge %s: %v", portName, bridgeName, err)
	}

	return nil
}

type BridgeMapping struct {
	physnet   string
	ovsBridge string
}

func (bm BridgeMapping) String() string {
	return fmt.Sprintf("%s:%s", bm.physnet, bm.ovsBridge)
}

type BridgeMappings []BridgeMapping

func (bms BridgeMappings) String() string {
	return strings.Join(Map(bms, func(bm BridgeMapping) string { return bm.String() }), ",")
}

func Map[T, V any](items []T, fn func(T) V) []V {
	result := make([]V, len(items))
	for i, t := range items {
		result[i] = fn(t)
	}
	return result
}

func configureBridgeMappings(ovnNodeName string, mappings ...BridgeMapping) error {
	mappingsString := fmt.Sprintf("external_ids:ovn-bridge-mappings=%s", BridgeMappings(mappings).String())
	_, err := provider.Get().ExecK8NodeCommand(ovnNodeName, []string{"ovs-vsctl", "set", "open", ".", mappingsString})
	if err != nil {
		return fmt.Errorf("failed to configure bridge mapping(s): %v", err)
	}

	return err
}

func defaultNetworkBridgeMapping() BridgeMapping {
	return BridgeMapping{
		physnet:   "physnet",
		ovsBridge: "breth0", // FIXME breth0?
	}
}

func bridgeMapping(physnet, ovsBridge string) BridgeMapping {
	return BridgeMapping{
		physnet:   physnet,
		ovsBridge: ovsBridge,
	}
}
