// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package storage

import (
	"context"
	"fmt"
	"io"
	"os"

	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
)

type Type = setting.StorageType

// NewStorageFunc is a function that creates a storage
type NewStorageFunc func(ctx context.Context, cfg *setting.Storage) (ObjectStorage, error)

var storageMap = map[Type]NewStorageFunc{}

// RegisterStorageType registers a provided storage type with a function to create it
func RegisterStorageType(typ Type, fn func(ctx context.Context, cfg *setting.Storage) (ObjectStorage, error)) {
	storageMap[typ] = fn
}

// Object represents the object on the storage
type Object interface {
	io.ReadCloser
	io.Seeker
	Stat() (os.FileInfo, error)
}

// ObjectStorage represents an object storage to handle a bucket and files
type ObjectStorage interface {
	Open(path string) (Object, error)

	// Save store an object, if size is unknown set -1
	// NOTICE: Some storage SDK will close the Reader after saving if it is also a Closer,
	// DO NOT use the reader anymore after Save, or wrap it to a non-Closer reader.
	Save(path string, r io.Reader, size int64) (int64, error)

	Stat(path string) (os.FileInfo, error)
	Delete(path string) error

	// IterateObjects calls the iterator function for each object in the storage with the given path as prefix
	// The "fullPath" argument in callback is the full path in this storage.
	// * IterateObjects("", ...): iterate all objects in this storage
	// * IterateObjects("sub-path", ...): iterate all objects with "sub-path" as prefix in this storage, the "fullPath" will be like "sub-path/xxx"
	IterateObjects(basePath string, iterator func(fullPath string, obj Object) error) error
}

// Copy copies a file from source ObjectStorage to dest ObjectStorage
func Copy(dstStorage ObjectStorage, dstPath string, srcStorage ObjectStorage, srcPath string) (int64, error) {
	f, err := srcStorage.Open(srcPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	size := int64(-1)
	fsinfo, err := f.Stat()
	if err == nil {
		size = fsinfo.Size()
	}

	return dstStorage.Save(dstPath, f, size)
}

// Clean delete all the objects in this storage
func Clean(storage ObjectStorage) error {
	return storage.IterateObjects("", func(path string, obj Object) error {
		_ = obj.Close()
		return storage.Delete(path)
	})
}

// LFS represents lfs storage
var LFS ObjectStorage = uninitializedStorage

// Init init the storage
func Init() error {
	return initLFS()
}

// NewStorage takes a storage type and some config and returns an ObjectStorage or an error
func NewStorage(typStr Type, cfg *setting.Storage) (ObjectStorage, error) {
	if len(typStr) == 0 {
		typStr = setting.LocalStorageType
	}
	fn, ok := storageMap[typStr]
	if !ok {
		return nil, fmt.Errorf("Unsupported storage type: %s", typStr)
	}

	return fn(context.Background(), cfg)
}

func initLFS() (err error) {
	if !setting.LFS.StartServer {
		LFS = discardStorage("LFS isn't enabled")
		return nil
	}
	log.Info("Initialising LFS storage with type: %s", setting.LFS.Storage.Type)
	LFS, err = NewStorage(setting.LFS.Storage.Type, setting.LFS.Storage)
	return err
}
