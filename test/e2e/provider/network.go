package provider

type Network struct {
	Name    string
	Configs []NetworkConfig
}

type NetworkConfig struct {
	Subnet  string `json:"Subnet"`
	Gateway string `json:"Gateway"`
}

func (n Network) Equal(candidate Network) bool {
	if n.Name != candidate.Name {
		return false
	}
	return true
}

func (n Network) String() string {
	return n.Name
}

type Networks struct {
	List []Network
}

func (n *Networks) InsertNoDupe(candidate Network) {
	var found bool
	for _, network := range n.List {
		if network.Equal(candidate) {
			found = true
			break
		}
	}
	if !found {
		n.List = append(n.List, candidate)
	}
}

type networkAttachment struct {
	network Network
	node    string
}

func (na networkAttachment) equal(candidate networkAttachment) bool {
	if na.node != candidate.node {
		return false
	}
	if !na.network.Equal(candidate.network) {
		return false
	}
	return true
}

type NetworkAttachments struct {
	List []networkAttachment
}

func (na *NetworkAttachments) insertNoDupe(candidate networkAttachment) {
	var found bool
	for _, existingNetworkAttachment := range na.List {
		if existingNetworkAttachment.equal(candidate) {
			found = true
			break
		}
	}
	if !found {
		na.List = append(na.List, candidate)
	}
}

type NetworkInterface struct {
	IPv4Gateway string
	IPv4        string
	IPv4Prefix  string
	IPv6Gateway string
	IPv6        string
	IPv6Prefix  string
	MAC         string
}
