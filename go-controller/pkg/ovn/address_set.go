package ovn

import (
	"fmt"
	"strings"

	"github.com/ovn-org/ovn-kubernetes/go-controller/pkg/util"

	"k8s.io/klog"
)

// An AddressSetRef provides the DisplayName and HashedName of an address set, as well as
// the components from which the DisplayName is made.
//
// For backward-compatibility reasons, there are three possible formats for DisplayName:
//
//   - If OwnerType is "Namespace", IPv6 is false, and OwnerName and Args are empty, then
//     DisplayName is just the same as OwnerNamespace.
//
//   - If OwnerType is "NetworkPolicy", and IPv6 is false, then DisplayName is
//     "OwnerNamespace.OwnerName.Args..."
//
//   - Otherwise, DisplayName is ".OwnerType.IPFamily.OwnerNamespace.OwnerName.Args..."
type AddressSetRef struct {
	OwnerType      string
	OwnerNamespace string
	OwnerName      string
	IPv6           bool
	Args           []string

	DisplayName string
	HashedName  string
}

// newAddressSetRef creates an AddressSetRef based on the arguments
func newAddressSetRef(ownerType string, ipv6 bool, ownerNamespace, ownerName string, args ...string) *AddressSetRef {
	ref := &AddressSetRef{
		OwnerType:      ownerType,
		OwnerNamespace: ownerNamespace,
		OwnerName:      ownerName,
		IPv6:           ipv6,
		Args:           args,
	}

	var nameParts []string
	if ownerType == "Namespace" && !ipv6 && ownerName == "" && args == nil {
		nameParts = append(nameParts, ownerNamespace)
	} else if ownerType == "NetworkPolicy" && !ipv6 {
		nameParts = append(nameParts, ownerNamespace, ownerName)
		nameParts = append(nameParts, args...)
	} else {
		nameParts = append(nameParts, "", ownerType)
		if ipv6 {
			nameParts = append(nameParts, "v6")
		} else {
			nameParts = append(nameParts, "v4")
		}
		nameParts = append(nameParts, ownerNamespace, ownerName)
		nameParts = append(nameParts, args...)
	}

	ref.DisplayName = strings.Join(nameParts, ".")
	ref.HashedName = hashForOVN(ref.DisplayName)
	return ref
}

// parseAddressSetRef parses an AddressSetRef from its display name (returning nil on error)
func parseAddressSetRef(displayName string) (*AddressSetRef) {
	ref := &AddressSetRef{
		DisplayName: displayName,
		HashedName:  hashForOVN(displayName),
	}

	nameParts := strings.Split(displayName, ".")
	if len(nameParts) > 5 && nameParts[0] == "" {
		ref.OwnerType = nameParts[1]
		ref.IPv6 = nameParts[2] == "v6"
		ref.OwnerNamespace = nameParts[3]
		ref.OwnerName = nameParts[4]
		ref.Args = nameParts[5:]
	} else if len(nameParts) > 1 && nameParts[0] != "" {
		ref.OwnerType = "NetworkPolicy"
		ref.IPv6 = false
		ref.OwnerNamespace = nameParts[0]
		ref.OwnerName = nameParts[1]
		ref.Args = nameParts[2:]
	} else if len(nameParts) == 1 && nameParts[0] != "" {
		ref.OwnerType = "Namespace"
		ref.IPv6 = false
		ref.OwnerNamespace = nameParts[0]
	} else {
		return nil
	}

	return ref
}

// forEachAddressSet calls iteratorFn for every address_set in OVN
func (oc *Controller) forEachAddressSet(iteratorFn func(*AddressSetRef)) error {
	output, stderr, err := util.RunOVNNbctl("--data=bare", "--no-heading",
		"--columns=external_ids", "find", "address_set")
	if err != nil {
		klog.Errorf("Error in obtaining list of address sets from OVN: "+
			"stdout: %q, stderr: %q err: %v", output, stderr, err)
		return err
	}
	for _, addrSet := range strings.Fields(output) {
		if !strings.HasPrefix(addrSet, "name=") {
			continue
		}
		ref := parseAddressSetRef(addrSet[5:])
		if ref == nil {
			klog.Warningf("Could not parse address set %q; ignoring", addrSet)
			continue
		}
		iteratorFn(ref)
	}
	return nil
}

func addToAddressSet(ref *AddressSetRef, address string) {
	klog.V(5).Infof("addToAddressSet %s with %s", ref.DisplayName, address)

	_, stderr, err := util.RunOVNNbctl("add", "address_set",
		ref.HashedName, "addresses", `"`+address+`"`)
	if err != nil {
		klog.Errorf("failed to add an address %q to address_set %q, stderr: %q (%v)",
			address, ref.DisplayName, stderr, err)
	}
}

func removeFromAddressSet(ref *AddressSetRef, address string) {
	klog.V(5).Infof("removeFromAddressSet %s with %s", ref.DisplayName, address)

	_, stderr, err := util.RunOVNNbctl("remove", "address_set",
		ref.HashedName, "addresses", `"`+address+`"`)
	if err != nil {
		klog.Errorf("failed to remove an address %q from address_set %q, stderr: %q (%v)",
			address, ref.DisplayName, stderr, err)
	}
}

func createAddressSet(ref *AddressSetRef, addresses []string) {
	klog.V(5).Infof("createAddressSet with %s and %s", ref.DisplayName, addresses)
	addressSet, stderr, err := util.RunOVNNbctl("--data=bare",
		"--no-heading", "--columns=_uuid", "find", "address_set",
		fmt.Sprintf("name=%s", ref.HashedName))
	if err != nil {
		klog.Errorf("find failed to get address set, stderr: %q (%v)",
			stderr, err)
		return
	}

	// addressSet has already been created in the database and nothing to set.
	if addressSet != "" && len(addresses) == 0 {
		_, stderr, err = util.RunOVNNbctl("clear", "address_set",
			ref.HashedName, "addresses")
		if err != nil {
			klog.Errorf("failed to clear address_set, stderr: %q (%v)",
				stderr, err)
		}
		return
	}

	ips := `"` + strings.Join(addresses, `" "`) + `"`

	// An addressSet has already been created. Just set addresses.
	if addressSet != "" {
		// Set the addresses
		_, stderr, err = util.RunOVNNbctl("set", "address_set",
			ref.HashedName, fmt.Sprintf("addresses=%s", ips))
		if err != nil {
			klog.Errorf("failed to set address_set, stderr: %q (%v)",
				stderr, err)
		}
		return
	}

	// addressSet has not been created yet. Create it.
	if len(addresses) == 0 {
		_, stderr, err = util.RunOVNNbctl("create", "address_set",
			fmt.Sprintf("name=%s", ref.HashedName),
			fmt.Sprintf("external-ids:name=%s", ref.DisplayName))
	} else {
		_, stderr, err = util.RunOVNNbctl("create", "address_set",
			fmt.Sprintf("name=%s", ref.HashedName),
			fmt.Sprintf("external-ids:name=%s", ref.DisplayName),
			fmt.Sprintf("addresses=%s", ips))
	}
	if err != nil {
		klog.Errorf("failed to create address_set %s, stderr: %q (%v)",
			ref.DisplayName, stderr, err)
	}
}

func deleteAddressSet(ref *AddressSetRef) {
	klog.V(5).Infof("deleteAddressSet %s", ref.DisplayName)

	_, stderr, err := util.RunOVNNbctl("--if-exists", "destroy",
		"address_set", ref.HashedName)
	if err != nil {
		klog.Errorf("failed to destroy address set %s, stderr: %q, (%v)",
			ref.DisplayName, stderr, err)
		return
	}
}
