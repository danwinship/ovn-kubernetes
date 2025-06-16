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
func DeleteObjects(ctx context.Context, objects []knftables.Object) error {
	nft, err := GetNFTablesHelper()
	if err != nil {
		return err
	}

	tx := nft.NewTransaction()
	for _, obj := range objects {
		tx.Destroy(obj)
	}
	return nft.Run(ctx, tx)
}

// SyncObjects synchronizes the given nftables containers to contain only the elements in
// contents. Currently containers must contain only Sets and Maps, while contents must
// contain only Elements.
func SyncObjects(ctx context.Context, containers, contents []knftables.Object) error {
	nft, err := GetNFTablesHelper()
	if err != nil {
		return err
	}

	tx := nft.NewTransaction()

	syncContainers := make(map[string]knftables.Object)
	for _, obj := range containers {
		switch typed := obj.(type) {
		case *knftables.Set:
			syncContainers[typed.Name] = obj
		case *knftables.Map:
			syncContainers[typed.Name] = obj
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
	return nil
}
