package provider

import (
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/client-go/rest"
)

type Provider interface {
	Name() string
	NewTestContext() Context

	PrimaryNetwork() (Network, error)
	PrimaryInterfaceName() string

	GetExternalContainerNetworkInterface(container ExternalContainer, network Network) (NetworkInterface, error)
	GetK8NodeNetworkInterface(instance string, network Network) (NetworkInterface, error)

	ExecK8NodeCommand(nodeName string, cmd []string) (string, error)
	ExecExternalContainerCommand(container ExternalContainer, cmd []string) (string, error)
	GetExternalContainerLogs(container ExternalContainer) (string, error)
	GetExternalContainerPort() int32
	GetK8HostPort() int32
}

type Context interface {
	CreateExternalContainer(container ExternalContainer) (ExternalContainer, error)
	DeleteExternalContainer(container ExternalContainer) error

	CreateNetwork(name string, subnets ...string) (Network, error)
	DeleteNetwork(network Network) error
	AttachNetwork(network Network, instance string) (NetworkInterface, error)
	DetachNetwork(network Network, instance string) error

	AddCleanUpFn(func() error)
}

type Name string

func (n Name) String() string {
	return string(n)
}

var provider Provider

func Set(config *rest.Config) {
	// detect if the provider is KinD
	if isKind() {
		provider = newKinDProvider()
	}
	if isOpenShift() {
		provider = newOpenShiftProvider(config)
	}
	if provider == nil {
		panic("failed to determine the infrastructure provider")
	}
}

func Get() Provider {
	if provider == nil {
		panic("provider not set")
	}
	return provider
}

func isKind() bool {
	_, err := exec.LookPath("kind")
	if err != nil {
		return false
	}
	outBytes, err := exec.Command("kind", "get", "clusters").CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("failed to get KinD clusters: stdout: %q, err: %v", string(outBytes), err))
	}
	if strings.Contains(string(outBytes), "ovn") {
		return true
	}
	return false
}

func isOpenShift() bool {
	_, err := exec.LookPath("kubectl")
	if err != nil {
		panic("failed to find kubectl in PATH")
	}
	crdName := "infrastructures.config.openshift.io"
	outBytes, err := exec.Command("kubectl", "get", "crd").CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("failed to list CRDs: %v", err))
	}
	return strings.Contains(string(outBytes), crdName)
}
