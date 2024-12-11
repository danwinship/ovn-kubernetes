//go:build linux
// +build linux

package nftables

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/knftables"
)

// AddObjects adds each element of objects to nftables in a single transaction, using
// tx.Add() for each object.
func AddObjects(ctx context.Context, objects []knftables.Object) error {
	nft, err := GetNFTablesHelper()
	if err != nil {
		return err
	}

	tx := nft.NewTransaction()
	for _, obj := range objects {
		tx.Add(obj)
	}
	return nft.Run(ctx, tx)
}

// DeleteObjects deletes each element of objects from nftables, if it exists; no errors
// are returned for objects that don't exist.
//
// For knftables.Rule objects, if the Rule's Handle is not set, then it must have a
// non-nil Comment. DeleteObjects will first "list" the Rule's Chain to find its current
// contents, and then delete every rule in the chain with a matching comment.
func DeleteObjects(ctx context.Context, objects []knftables.Object) error {
	nft, err := GetNFTablesHelper()
	if err != nil {
		return err
	}

	// List existing objects we need to list
	existingChains := make(map[string][]*knftables.Rule)
	for _, obj := range objects {
		if typed, ok := obj.(*knftables.Rule); ok {
			if typed.Handle != nil {
				continue
			} else if typed.Comment == nil {
				return fmt.Errorf("Rule passed to DeleteObject must have a Handle or Comment")
			}
			if existingChains[typed.Chain] == nil {
				rules, err := nft.ListRules(ctx, typed.Chain)
				if err != nil {
					return err
				}
				existingChains[typed.Chain] = rules
			}
		}
	}

	// Now build the actual transaction
	tx := nft.NewTransaction()
	for _, obj := range objects {
		if typed, ok := obj.(*knftables.Rule); ok && typed.Handle == nil {
			for _, rule := range findRulesByComment(existingChains[typed.Chain], *typed.Comment) {
				tx.Destroy(rule)
			}
		} else {
			tx.Destroy(obj)
		}
	}
	return nft.Run(ctx, tx)
}

func findRulesByComment(rules []*knftables.Rule, comment string) []*knftables.Rule {
	matches := make([]*knftables.Rule, 0, 1)
	for _, rule := range rules {
		if *rule.Comment == comment {
			matches = append(matches, rule)
		}
	}
	return matches
}

// SyncObjects synchronizes the given nftables containers to contain only the elements in
// contents. containers can contain Sets, Maps, and Chains, and contents can contain
// Elements and Rules.
func SyncObjects(ctx context.Context, containers, contents []knftables.Object) error {
	nft, err := GetNFTablesHelper()
	if err != nil {
		return err
	}

	tx := nft.NewTransaction()

	syncContainers := make(map[string]knftables.Object)
	syncChains := make(map[string]*knftables.Chain)
	for _, obj := range containers {
		switch typed := obj.(type) {
		case *knftables.Set:
			syncContainers[typed.Name] = obj
		case *knftables.Map:
			syncContainers[typed.Name] = obj
		case *knftables.Chain:
			syncChains[typed.Name] = typed
		default:
			return fmt.Errorf("unsupported container type %T passed to SyncObjects", obj)
		}
		tx.Flush(obj)
	}

	for _, obj := range contents {
		switch typed := obj.(type) {
		case *knftables.Element:
			if typed.Set != "" && syncContainers[typed.Set] == nil {
				return fmt.Errorf("unexpected element from set %q which is not in containers", typed.Set)
			} else if typed.Map != "" && syncContainers[typed.Map] == nil {
				return fmt.Errorf("unexpected element from map %q which is not in containers", typed.Map)
			}
		case *knftables.Rule:
			if syncChains[typed.Chain] == nil {
				return fmt.Errorf("unexpected rule from chain %q which is not in containers", typed.Chain)
			}
		default:
			return fmt.Errorf("unsupported contents type %T passed to SyncObjects", obj)
		}
		tx.Add(obj)
	}

	err = nft.Run(ctx, tx)
	if err == nil || !knftables.IsNotFound(err) {
		return err
	}

	// For compatibility with
	// https://github.com/ovn-kubernetes/ovn-kubernetes/pull/5250, try again, doing
	// each container separately, ignoring errors if we are asked to make a set/map
	// empty when the set/map doesn't actually exist
	for _, containerName := range sets.List(sets.KeySet(syncContainers)) {
		tx := nft.NewTransaction()
		tx.Flush(syncContainers[containerName])
		keepElems := 0
		for _, obj := range contents {
			switch typed := obj.(type) {
			case *knftables.Element:
				if typed.Set == containerName || typed.Map == containerName {
					tx.Add(obj)
					keepElems++
				}
			}
		}
		err := nft.Run(ctx, tx)
		if err != nil && (!knftables.IsNotFound(err) || keepElems > 0) {
			return err
		}
	}

	for _, chainName := range sets.List(sets.KeySet(syncChains)) {
		tx := nft.NewTransaction()
		tx.Flush(syncChains[chainName])
		keepRules := 0
		for _, obj := range contents {
			switch typed := obj.(type) {
			case *knftables.Rule:
				if typed.Chain == chainName {
					tx.Add(obj)
					keepRules++
				}
			}
		}
		err := nft.Run(ctx, tx)
		if err != nil && (!knftables.IsNotFound(err) || keepRules > 0) {
			return err
		}
	}

	return nil
}
