package ovn

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Address Sets", func() {
	It("generates backward-compatible hashed address set names", func() {
		// Address Set name for an IPv4 Namespace is just the namespace name
		nsRef := newAddressSetRef("Namespace", false, "testing", "")
		nsHashed := hashedAddressSet("testing")
		Expect(nsRef.DisplayName).To(Equal("testing"))
		Expect(nsRef.HashedName).To(Equal(nsHashed))

		// Specifying both ownerNamespace and ownerName changes to a new-style reference
		nsRefNamed := newAddressSetRef("Namespace", false, "testing", "also-testing")
		Expect(nsRefNamed.DisplayName).To(Equal(".Namespace.v4.testing.also-testing"))
		Expect(nsRefNamed.HashedName).NotTo(Equal(nsRef.HashedName))

		// Adding extra args changes the reference
		nsRefArgs := newAddressSetRef("Namespace", false, "testing", "", "blah", "blah")
		Expect(nsRefArgs.DisplayName).To(Equal(".Namespace.v4.testing..blah.blah"))
		Expect(nsRefArgs.HashedName).NotTo(Equal(nsRef.HashedName))
		Expect(nsRefArgs.HashedName).NotTo(Equal(nsRefNamed.HashedName))

		// Specifying IPv6 changes the reference
		nsRef6 := newAddressSetRef("Namespace", true, "testing", "")
		Expect(nsRef6.DisplayName).To(Equal(".Namespace.v6.testing."))
		Expect(nsRef6.HashedName).NotTo(Equal(nsRef.HashedName))
		Expect(nsRef6.HashedName).NotTo(Equal(nsRefNamed.HashedName))
		Expect(nsRef6.HashedName).NotTo(Equal(nsRefArgs.HashedName))

		// Address Set name for an IPv4 NetworkPolicy does not include type/family
		npRef := newAddressSetRef("NetworkPolicy", false, "testing", "policy", "ingress", "1")
		npHashed := hashedAddressSet("testing.policy.ingress.1")
		Expect(npRef.DisplayName).To(Equal("testing.policy.ingress.1"))
		Expect(npRef.HashedName).To(Equal(npHashed))

		// But for IPv6 it does
		npRef6 := newAddressSetRef("NetworkPolicy", true, "testing", "policy", "ingress", "1")
		Expect(npRef6.DisplayName).To(Equal(".NetworkPolicy.v6.testing.policy.ingress.1"))
		Expect(npRef6.HashedName).NotTo(Equal(npHashed))
	})
})

// for backward-compatibility in existing tests
func hashedAddressSet(s string) string {
	return hashForOVN(s)
}
