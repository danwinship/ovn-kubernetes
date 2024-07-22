package provider

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/ovn-org/ovn-kubernetes/test/e2e/containerruntime"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	utilnet "k8s.io/utils/net"
)

type kind struct {
	externalContainerPort *portAllocator
	hostPort              *portAllocator
}

func newKinDProvider() Provider {
	return &kind{externalContainerPort: newPortAllocator(12000, 65535), hostPort: newPortAllocator(1024, 65535)}
}

func (k *kind) Name() string {
	return "kind"
}

func (k *kind) PrimaryNetwork() (Network, error) {
	return getContainerNetwork("kind")
}

func (k *kind) PrimaryInterfaceName() string {
	return "eth0"
}

func (k *kind) GetExternalContainerNetworkInterface(container ExternalContainer, network Network) (NetworkInterface, error) {
	return getContainerNetworkInterface(container.Name, network)
}

func (k *kind) GetK8NodeNetworkInterface(instance string, network Network) (NetworkInterface, error) {
	return getContainerNetworkInterface(instance, network)
}

func (k *kind) ExecK8NodeCommand(nodeName string, cmd []string) (string, error) {
	if len(cmd) == 0 {
		panic("ExecK8NodeCommand(): insufficient command arguments")
	}
	cmdArgs := append([]string{"exec", nodeName}, cmd...)
	output, err := exec.Command(containerruntime.Get().String(), cmdArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %s (%s)", strings.Join(cmd, " "), err, output)
	}
	return string(output), nil
}

func (k *kind) ExecExternalContainerCommand(container ExternalContainer, cmd []string) (string, error) {
	cmdArgs := append([]string{"exec", container.Name}, cmd...)
	out, err := exec.Command(containerruntime.Get().String(), cmdArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to exec container command (%s): err: %v, stdout: %q", strings.Join(cmdArgs, " "), err, out)
	}
	return string(out), nil
}

func (k *kind) GetExternalContainerLogs(container ExternalContainer) (string, error) {
	// check if it is present before retrieving logs
	output, err := exec.Command(containerruntime.Get().String(), "ps", "-f", fmt.Sprintf("Name=^%s$", container.Name), "-q").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to check if external container (%s) exists: %v (%s)", container, err, output)
	}
	if string(output) == "" {
		return "", fmt.Errorf("external container (%s) does not exist", container.String())
	}
	output, err = exec.Command(containerruntime.Get().String(), "logs", container.Name).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs of external container (%s): %v (%s)", container, err, output)
	}
	return string(output), nil
}

func (k *kind) GetExternalContainerPort() int32 {
	return k.externalContainerPort.allocate()
}

func (k *kind) GetK8HostPort() int32 {
	return k.hostPort.allocate()
}

func (k *kind) NewTestContext() Context {
	ck := &contextKind{}
	ginkgo.DeferCleanup(ck.CleanUp)
	return ck
}

type contextKind struct {
	cleanUpNetworkAttachments NetworkAttachments
	cleanUpNetworks           Networks
	cleanUpContainers         []ExternalContainer
	cleanUpFns                []func() error
}

func (c *contextKind) CreateExternalContainer(container ExternalContainer) (ExternalContainer, error) {
	if valid, err := container.isValidPreCreate(); !valid {
		return container, err
	}

	cmdArgs := []string{"run", "-itd", "--privileged", "--name", container.Name, "--network", container.Network.Name, "--hostname", container.Name,
		container.Image}
	cmdArgs = append(cmdArgs, container.CMD...)
	fmt.Printf("creating container with command: %q\n", strings.Join(cmdArgs, " "))
	output, err := exec.Command(containerruntime.Get().String(), cmdArgs...).CombinedOutput()
	if err != nil {
		return container, fmt.Errorf("failed to create container %s: %s (%s)", container, err, output)
	}
	ipv4Bytes, err := exec.Command(containerruntime.Get().String(), "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", container.Name).CombinedOutput()
	if err != nil {
		return container, fmt.Errorf("failed to retrieve IPv4 address from container %s: %s (%s)", container, err, output)
	}
	ipv6Bytes, err := exec.Command(containerruntime.Get().String(), "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.GlobalIPv6Address}}{{end}}", container.Name).CombinedOutput()
	if err != nil {
		return container, fmt.Errorf("failed to retrieve IPv6 address from container %s: %s (%s)", container, err, output)
	}
	container.ipv4 = strings.Trim(string(ipv4Bytes), "\n")
	container.ipv6 = strings.Trim(string(ipv6Bytes), "\n")
	if valid, err := container.isValidPostCreate(); !valid {
		return container, err
	}
	c.cleanUpContainers = append(c.cleanUpContainers, container)
	return container, nil
}

func (c *contextKind) DeleteExternalContainer(container ExternalContainer) error {
	// check if it is present before deleting
	output, err := exec.Command(containerruntime.Get().String(), "ps", "-f", fmt.Sprintf("Name=^%s$", container.Name), "-q").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check if external container (%s) is already deleted: %v (%s)", container, err, output)
	}
	if string(output) == "" {
		return nil
	}
	output, err = exec.Command(containerruntime.Get().String(), "rm", "-f", container.Name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete external container (%s): %v (%s)", container, err, output)
	}
	err = wait.ExponentialBackoff(wait.Backoff{Duration: 1 * time.Second, Factor: 5, Steps: 5}, wait.ConditionFunc(func() (done bool, err error) {
		output, err = exec.Command(containerruntime.Get().String(), "ps", "-f", fmt.Sprintf("Name=^%s$", container.Name), "-q").CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("failed to check if external container (%s) is deleted: %v (%s)", container, err, output)
		}
		if string(output) != "" {
			return false, nil
		}
		return true, nil
	}))
	if err != nil {
		return fmt.Errorf("failed to delete external container (%s): %v", container, err)
	}
	return nil
}

func (c *contextKind) CreateNetwork(name string, subnets ...string) (Network, error) {
	network := Network{name, nil}
	cmdArgs := []string{"network", "create", "--internal", "--driver", "bridge", name}
	var v6 bool
	// detect if IPv6 flag is required
	for _, subnet := range subnets {
		cmdArgs = append(cmdArgs, "--subnet", subnet)
		if utilnet.IsIPv6CIDRString(subnet) {
			v6 = true
		}
	}
	if v6 {
		cmdArgs = append(cmdArgs, "--ipv6")
	}
	output, err := exec.Command(containerruntime.Get().String(), cmdArgs...).CombinedOutput()
	if err != nil {
		return network, fmt.Errorf("failed to create Network with command %q: %s (%s)", strings.Join(cmdArgs, " "), err, output)
	}
	c.cleanUpNetworks.InsertNoDupe(network)
	return getContainerNetwork(name)
}

func (c *contextKind) AttachNetwork(network Network, instance string) (NetworkInterface, error) {
	// return if the network is connected to the container
	output, err := exec.Command(containerruntime.Get().String(), "inspect", "-f", fmt.Sprintf(inspectNetwork, network.Name), instance).CombinedOutput()
	if err != nil {
		return NetworkInterface{}, fmt.Errorf("failed to check if network %s is already attached to instance %s: %v", network.Name, instance, err)
	}
	if strings.Trim(string(output), "\n") != emptyValue {
		return getContainerNetworkInterface(instance, network)
	}
	output, err = exec.Command(containerruntime.Get().String(), "network", "connect", network.Name, instance).CombinedOutput()
	if err != nil {
		return NetworkInterface{}, fmt.Errorf("failed to attach network to instance %s: %s (%s)", instance, err, output)
	}
	c.cleanUpNetworkAttachments.insertNoDupe(networkAttachment{network: network, node: instance})
	return getContainerNetworkInterface(instance, network)
}

func (c *contextKind) DetachNetwork(network Network, instance string) error {
	// return if the network isn't connected to the container
	output, err := exec.Command(containerruntime.Get().String(), "inspect", "-f", fmt.Sprintf(inspectNetwork, network.Name), instance).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check if network %s is attached to instance %s: %v", network.Name, instance, err)
	}
	if strings.Trim(string(output), "\n") == emptyValue {
		return nil
	}
	output, err = exec.Command(containerruntime.Get().String(), "network", "disconnect", network.Name, instance).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to detach network %s from node %s: %s (%s)", network, instance, err, output)
	}
	return nil
}

func (c *contextKind) DeleteNetwork(network Network) error {
	// check if it is present before deleting
	output, err := exec.Command(containerruntime.Get().String(), "network", "ls", "-f", fmt.Sprintf("Name=^%s$", network.Name), "-q").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check if network (%s) is already deleted: %v (%s)", network.Name, err, output)
	}
	if string(output) == "" {
		return nil
	}
	output, err = exec.Command(containerruntime.Get().String(), "network", "rm", network.Name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete network %s: %s (%s)", network.Name, err, output)
	}
	return nil
}

func (c *contextKind) AddCleanUpFn(cleanUpFn func() error) {
	c.cleanUpFns = append(c.cleanUpFns, cleanUpFn)
}

func (c *contextKind) CleanUp() error {
	var errs []error
	// generic cleanup activities
	for i := len(c.cleanUpFns) - 1; i >= 0; i-- {
		framework.Logf("CleanUp: exec cleanup func %d of %d", i+1, len(c.cleanUpFns))
		if err := c.cleanUpFns[i](); err != nil {
			errs = append(errs, err)
		}
	}
	c.cleanUpFns = nil
	// detach network(s) from nodes
	for _, na := range c.cleanUpNetworkAttachments.List {
		framework.Logf("CleanUp: detach network %s from node %s", na.network.Name, na.node)
		if err := c.DetachNetwork(na.network, na.node); err != nil {
			errs = append(errs, err)
		}
	}
	// remove containers
	for _, container := range c.cleanUpContainers {
		framework.Logf("CleanUp: deleting container %s", container.Name)
		if err := c.DeleteExternalContainer(container); err != nil {
			errs = append(errs, err)
		}
	}
	c.cleanUpContainers = nil
	// delete secondary networks
	for _, network := range c.cleanUpNetworks.List {
		framework.Logf("CleanUp: delete network %s", network.Name)
		if err := c.DeleteNetwork(network); err != nil {
			errs = append(errs, err)
		}
	}
	c.cleanUpNetworks.List = nil
	return condenseErrors(errs)
}

const (
	inspectNetworkConfigs       = "{{ json .IPAM.Config}}"
	inspectNetwork              = "{{.NetworkSettings.Networks.%s}}"
	inspectNetworkAttribute     = "{{.NetworkSettings.Networks.%s.%s}}"
	inspectNetworkIPv4GWKey     = "Gateway"
	inspectNetworkIPv4AddrKey   = "IPAddress"
	inspectNetworkIPv4PrefixKey = "IPPrefixLen"
	inspectNetworkIPv6GWKey     = "IPv6Gateway"
	inspectNetworkIPv6AddrKey   = "GlobalIPv6Address"
	inspectNetworkIPv6PrefixKey = "GlobalIPv6PrefixLen"
	inspectNetworkMACKey        = "MacAddress"
	emptyValue                  = "<no value>"
)

func getContainerNetwork(networkName string) (Network, error) {
	n := Network{Name: networkName}
	configs := make([]NetworkConfig, 0, 1)
	dataBytes, err := exec.Command(containerruntime.Get().String(), "network", "inspect", "-f", inspectNetworkConfigs, networkName).CombinedOutput()
	if err != nil {
		return n, fmt.Errorf("failed to extract network %q data: %v", networkName, err)
	}
	dataBytes = []byte(strings.Trim(string(dataBytes), "\n"))
	if err = json.Unmarshal(dataBytes, &configs); err != nil {
		return n, fmt.Errorf("failed to unmarshall kind network %q configuration: %v", networkName, err)
	}
	n.Configs = configs
	return n, nil
}

func getContainerNetworkInterface(containerName string, network Network) (NetworkInterface, error) {
	getKeysValue := func(key string) (string, error) {
		value, err := exec.Command(containerruntime.Get().String(), "inspect", "-f",
			fmt.Sprintf(inspectNetworkAttribute, network.Name, key), containerName).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to extract %s network data for container %s using key %s: %v", network.Name, containerName, key, err)
		}
		return strings.TrimSuffix(string(value), "\n"), nil
	}
	var err error
	var ni = NetworkInterface{}
	ni.IPv4Gateway, err = getKeysValue(inspectNetworkIPv4GWKey)
	if err != nil {
		return ni, err
	}
	ni.IPv6Prefix, err = getKeysValue(inspectNetworkIPv4AddrKey)
	if err != nil {
		return ni, err
	}
	ni.IPv6Gateway, err = getKeysValue(inspectNetworkIPv6GWKey)
	if err != nil {
		return ni, err
	}
	ni.IPv4Prefix, err = getKeysValue(inspectNetworkIPv4PrefixKey)
	if err != nil {
		return ni, err
	}
	ni.IPv6, err = getKeysValue(inspectNetworkIPv6AddrKey)
	if err != nil {
		return ni, err
	}
	ni.IPv6Prefix, err = getKeysValue(inspectNetworkIPv6PrefixKey)
	if err != nil {
		return ni, err
	}
	ni.MAC, err = getKeysValue(inspectNetworkMACKey)
	if err != nil {
		return ni, err
	}
	return ni, nil
}
