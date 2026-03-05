package node

import (
	"testing"

	nodenft "github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/node/nftables"
	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNodeSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	util.SetFakeIPTablesHelpers()
	nodenft.SetFakeNFTablesHelper()
	RunSpecs(t, "Node Suite")
}
