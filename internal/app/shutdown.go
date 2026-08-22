package app

import "github.com/VanceMichael/greengrid/internal/storage/sqlite"

func closeRuntimeStore(store *sqlite.Store) error {
	return store.Close()
}
