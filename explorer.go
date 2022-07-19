package explorer

import (
	"sync"

	"go.sia.tech/core/chain"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
)

// A Store is a database that stores information about elements, contracts,
// and blocks.
type Store interface {
	ChainStats(index types.ChainIndex) (ChainStats, error)
	SiacoinElement(id types.ElementID) (types.SiacoinElement, error)
	SiafundElement(id types.ElementID) (types.SiafundElement, error)
	FileContractElement(id types.ElementID) (types.FileContractElement, error)
	UnspentSiacoinElements(address types.Address) ([]types.ElementID, error)
	UnspentSiafundElements(address types.Address) ([]types.ElementID, error)
	Transaction(id types.TransactionID) (types.Transaction, error)
	Transactions(address types.Address, amount, offset int) ([]types.TransactionID, error)
	State(index types.ChainIndex) (context consensus.State, err error)

	AddSiacoinElement(sce types.SiacoinElement)
	AddSiafundElement(sfe types.SiafundElement)
	AddFileContractElement(fce types.FileContractElement)
	RemoveElement(id types.ElementID)
	AddChainStats(index types.ChainIndex, stats ChainStats)
	AddUnspentSiacoinElement(address types.Address, id types.ElementID)
	AddUnspentSiafundElement(address types.Address, id types.ElementID)
	RemoveUnspentSiacoinElement(address types.Address, id types.ElementID)
	RemoveUnspentSiafundElement(address types.Address, id types.ElementID)
	AddTransaction(txn types.Transaction, addresses []types.Address, block types.ChainIndex)
	AddState(index types.ChainIndex, context consensus.State)

	Size() (uint64, error)
	Commit() error
}

// A HashStore can read and write hashes for nodes in the log's tree structure.
type HashStore interface {
	Size() (uint64, error)
	Commit() error
	ModifyLeaf(elem types.StateElement) error
	MerkleProof(leafIndex uint64) ([]types.Hash256, error)
}

// An Explorer contains a database storing information about blocks, outputs,
// contracts.
type Explorer struct {
	db       Store
	mu       sync.Mutex
	tipStats ChainStats
	cs       consensus.State
	hs       HashStore
}

// ProcessChainApplyUpdate implements chain.Subscriber.
func (e *Explorer) ProcessChainApplyUpdate(cau *chain.ApplyUpdate, mayCommit bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.db.AddState(cau.Block.Header.Index(), cau.State)

	stats := ChainStats{
		Block:               cau.Block,
		ActiveContractCost:  e.tipStats.ActiveContractCost,
		ActiveContractCount: e.tipStats.ActiveContractCount,
		ActiveContractSize:  e.tipStats.ActiveContractSize,
		TotalContractCost:   e.tipStats.TotalContractCost,
		TotalContractSize:   e.tipStats.TotalContractSize,
		TotalRevisionVolume: e.tipStats.TotalRevisionVolume,
	}

	for _, txn := range cau.Block.Transactions {
		// get a unique list of all addresses involved in transaction
		addrMap := make(map[types.Address]struct{})
		for _, elem := range txn.SiacoinInputs {
			addrMap[elem.Parent.Address] = struct{}{}
		}
		for _, elem := range txn.SiacoinOutputs {
			addrMap[elem.Address] = struct{}{}
		}
		for _, elem := range txn.SiafundInputs {
			addrMap[elem.Parent.Address] = struct{}{}
		}
		for _, elem := range txn.SiafundOutputs {
			addrMap[elem.Address] = struct{}{}
		}
		addrs := make([]types.Address, 0, len(addrMap))
		for addr := range addrMap {
			addrs = append(addrs, addr)
		}
		e.db.AddTransaction(txn, addrs, cau.Block.Header.Index())
	}

	for _, elem := range cau.SpentSiacoins {
		e.db.RemoveElement(elem.ID)
		e.db.RemoveUnspentSiacoinElement(elem.Address, elem.ID)
		stats.SpentSiacoinsCount++
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cau.SpentSiafunds {
		e.db.RemoveElement(elem.ID)
		e.db.RemoveUnspentSiafundElement(elem.Address, elem.ID)
		stats.SpentSiafundsCount++
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cau.ResolvedFileContracts {
		e.db.RemoveElement(elem.ID)
		stats.ActiveContractCount--
		payout := elem.FileContract.RenterOutput.Value.Add(elem.FileContract.HostOutput.Value)
		stats.ActiveContractCost = stats.ActiveContractCost.Sub(payout)
		stats.ActiveContractSize -= elem.FileContract.Filesize
		e.hs.ModifyLeaf(elem.StateElement)
	}

	for _, elem := range cau.NewSiacoinElements {
		e.db.AddSiacoinElement(elem)
		e.db.AddUnspentSiacoinElement(elem.Address, elem.ID)
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cau.NewSiafundElements {
		e.db.AddSiafundElement(elem)
		e.db.AddUnspentSiafundElement(elem.Address, elem.ID)
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cau.RevisedFileContracts {
		e.db.AddFileContractElement(elem)
		stats.TotalContractSize += elem.FileContract.Filesize
		stats.TotalRevisionVolume += elem.FileContract.Filesize
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cau.NewFileContracts {
		e.db.AddFileContractElement(elem)
		payout := elem.FileContract.RenterOutput.Value.Add(elem.FileContract.HostOutput.Value)
		stats.ActiveContractCount++
		stats.ActiveContractCost = stats.ActiveContractCost.Add(payout)
		stats.ActiveContractSize += elem.FileContract.Filesize
		stats.TotalContractCost = stats.TotalContractCost.Add(payout)
		stats.TotalContractSize += elem.FileContract.Filesize
		e.hs.ModifyLeaf(elem.StateElement)
	}

	e.db.AddChainStats(cau.State.Index, stats)

	e.cs, e.tipStats = cau.State, stats
	if mayCommit {
		if err := e.hs.Commit(); err != nil {
			return err
		}
		return e.db.Commit()
	}

	return nil
}

// ProcessChainRevertUpdate implements chain.Subscriber.
func (e *Explorer) ProcessChainRevertUpdate(cru *chain.RevertUpdate) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, elem := range cru.SpentSiacoins {
		e.db.AddSiacoinElement(elem)
		e.db.AddUnspentSiacoinElement(elem.Address, elem.ID)
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cru.SpentSiafunds {
		e.db.AddSiafundElement(elem)
		e.db.AddUnspentSiafundElement(elem.Address, elem.ID)
		e.hs.ModifyLeaf(elem.StateElement)
	}
	for _, elem := range cru.ResolvedFileContracts {
		e.db.AddFileContractElement(elem)
		e.hs.ModifyLeaf(elem.StateElement)
	}

	for _, elem := range cru.NewSiacoinElements {
		e.db.RemoveElement(elem.ID)
		e.db.RemoveUnspentSiacoinElement(elem.Address, elem.ID)
	}
	for _, elem := range cru.NewSiafundElements {
		e.db.RemoveElement(elem.ID)
		e.db.RemoveUnspentSiafundElement(elem.Address, elem.ID)
	}
	for _, elem := range cru.RevisedFileContracts {
		e.db.RemoveElement(elem.ID)
	}
	for _, txn := range cru.Block.Transactions {
		for _, rev := range txn.FileContractRevisions {
			e.db.AddFileContractElement(rev.Parent)
			e.hs.ModifyLeaf(rev.Parent.StateElement)
		}
	}
	for _, elem := range cru.NewFileContracts {
		e.db.RemoveElement(elem.ID)
	}

	oldStats, err := e.ChainStats(cru.State.Index)
	if err != nil {
		return err
	}

	// update validation context
	e.cs, e.tipStats = cru.State, oldStats
	return e.db.Commit()
}

// NewExplorer creates a new explorer.
func NewExplorer(cs consensus.State, store Store, hashStore HashStore) *Explorer {
	return &Explorer{
		cs: cs,
		db: store,
		hs: hashStore,
	}
}
