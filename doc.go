// Package fkmongo is a thin generic repository layer over the official
// MongoDB Go driver (v2).
//
// Embed [BaseRepository] in a domain repository to get typed CRUD methods,
// and add collection-specific methods next to it:
//
//	type SupplierRepo struct {
//		fkmongo.BaseRepository[Supplier]
//	}
//
//	func NewSupplierRepo(db *mongo.Database) SupplierRepo {
//		return SupplierRepo{fkmongo.NewBaseRepository[Supplier](db.Collection("suppliers"), "supplier")}
//	}
//
// Every method maps driver errors onto [ErrNotFound] and [ErrDuplicate], so
// callers can branch with errors.Is without importing the driver.
package fkmongo
