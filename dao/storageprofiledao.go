package dao

import (
	"github.com/skyrings/skyring/models"
)

type StorageProfileInterface interface {
	StorageProfile(name string) (sProfile models.StorageProfile, e error)
	StorageProfiles(query interface{}, ops models.QueryOps) (sProfiles []models.StorageProfile, e error)
	SaveStorageProfile(s models.StorageProfile) error
	DeleteStorageProfile(name string) error
}
