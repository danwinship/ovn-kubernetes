//go:build linux
// +build linux

package nftables

import (
	"context"

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
