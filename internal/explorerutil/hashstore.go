package explorerutil

import (
	"errors"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"

	"go.sia.tech/core/types"
)

type HashStore struct {
	hashFiles [64]*os.File
	numLeaves uint64
}

const hashSize = 32

// MerkleProof implements explorer.HashStore.
func (hs *HashStore) MerkleProof(leafIndex uint64) ([]types.Hash256, error) {
	pos := leafIndex
	proof := make([]types.Hash256, bits.Len64(leafIndex^hs.numLeaves)-1)
	for i := range proof {
		subtreeSize := uint64(1 << i)
		if leafIndex&(1<<i) == 0 {
			pos += subtreeSize
		} else {
			pos -= subtreeSize
		}
		if _, err := hs.hashFiles[i].ReadAt(proof[i][:], int64(pos/subtreeSize)*hashSize); err != nil {
			return nil, err
		}
	}
	return proof, nil
}

// ModifyLeaf overwrites hashes in the tree with the proof hashes in the
// provided element.
func (hs *HashStore) ModifyLeaf(elem types.StateElement) error {
	pos := elem.LeafIndex
	for i, h := range elem.MerkleProof {
		n := uint64(1 << i)
		subtreeSize := uint64(1 << i)
		if elem.LeafIndex&(1<<i) == 0 {
			pos += subtreeSize
		} else {
			pos -= subtreeSize
		}
		if _, err := hs.hashFiles[i].WriteAt(h[:], int64(pos/n)*hashSize); err != nil {
			return err
		}
	}
	if elem.LeafIndex+1 > hs.numLeaves {
		hs.numLeaves = elem.LeafIndex + 1
	}
	return nil
}

// Commit implements explorer.HashStore.
func (hs *HashStore) Commit() error {
	for _, f := range hs.hashFiles {
		if err := f.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// NewHashStore returns a new HashStore.
func NewHashStore(dir string) (*HashStore, error) {
	var hs HashStore
	for i := range hs.hashFiles {
		f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("tree_level_%d.dat", i)), os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return nil, err
		}
		stat, err := f.Stat()
		if err != nil {
			return nil, err
		} else if stat.Size()%hashSize != 0 {
			// TODO: attempt to repair automatically
			return nil, errors.New("tree contains a partially-written hash")
		}
		if i == 0 {
			hs.numLeaves = uint64(stat.Size()) / hashSize
		}
		hs.hashFiles[i] = f
	}
	return &hs, nil
}
