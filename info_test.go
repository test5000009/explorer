package explorer_test

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"

	"go.sia.tech/core/chain"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/explorer"
	"go.sia.tech/explorer/internal/chainutil"
	"go.sia.tech/explorer/internal/explorerutil"
	"go.sia.tech/explorer/internal/walletutil"
)

func testingKeypair(seed uint64) (types.PublicKey, types.PrivateKey) {
	var b [32]byte
	binary.LittleEndian.PutUint64(b[:], seed)
	privkey := types.NewPrivateKeyFromSeed(b)
	return privkey.PublicKey(), privkey
}

func addGenesisElements(e *explorer.Explorer, block types.Block) error {
	return e.ProcessChainApplyUpdate(&chain.ApplyUpdate{
		ApplyUpdate: consensus.GenesisUpdate(block, types.Work{NumHashes: [32]byte{31: 4}}),
		Block:       block,
	}, true)
}

func TestSiacoinElements(t *testing.T) {
	sim := chainutil.NewChainSim()
	cm := chain.NewManager(chainutil.NewEphemeralStore(sim.Genesis), sim.State)

	hs, err := explorerutil.NewHashStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	explorerStore := explorerutil.NewEphemeralStore()
	e := explorer.NewExplorer(sim.Genesis.State, explorerStore, hs)
	cm.AddSubscriber(e, cm.Tip())
	if err := addGenesisElements(e, sim.Genesis.Block); err != nil {
		t.Fatal(err)
	}

	w := walletutil.NewTestingWallet(cm.TipState())
	cm.AddSubscriber(w, cm.Tip())

	// fund the wallet with 100 coins
	ourAddr := w.NewAddress()
	fund := types.SiacoinOutput{Value: types.Siacoins(100), Address: ourAddr}
	if err := cm.AddTipBlock(sim.MineBlockWithSiacoinOutputs(fund)); err != nil {
		t.Fatal(err)
	}

	// wallet should now have a transaction, one element, and a non-zero balance

	// mine 5 blocks, each containing a transaction that sends some coins to
	// the void and some to ourself
	for i := 0; i < 5; i++ {
		sendAmount := types.Siacoins(7)
		txn := types.Transaction{
			SiacoinOutputs: []types.SiacoinOutput{{
				Address: types.VoidAddress,
				Value:   sendAmount,
			}},
		}

		if err := w.FundAndSign(&txn); err != nil {
			t.Fatal(err)
		}

		if err := cm.AddTipBlock(sim.MineBlockWithTxns(txn)); err != nil {
			t.Fatal(err)
		}

		changeAddr := txn.SiacoinOutputs[len(txn.SiacoinOutputs)-1].Address

		balance, err := e.SiacoinBalance(changeAddr)
		if err != nil {
			t.Fatal(err)
		}
		if !w.Balance().Equals(balance) {
			t.Fatal("balances don't equal")
		}

		outputs, err := e.UnspentSiacoinElements(changeAddr)
		if err != nil {
			t.Fatal(err)
		}
		if len(outputs) != 1 {
			t.Fatal("wrong amount of outputs")
		}

		elem, err := e.SiacoinElement(outputs[0])
		if err != nil {
			t.Fatal(err)
		}
		if !w.Balance().Equals(elem.Value) {
			t.Fatal("output value doesn't equal balance")
		}

		if elem.MerkleProof, err = e.MerkleProof(elem.ID); err != nil {
			t.Fatal(err)
		}

		cs := cm.TipState()
		if !cs.Elements.ContainsUnspentSiacoinElement(elem) {
			t.Fatal("accumulator should have unspent output")
		} else if cs.Elements.ContainsSpentSiacoinElement(elem) {
			t.Fatal("accumulator should not see output as spent")
		}

		txns, err := e.Transactions(changeAddr, math.MaxInt64, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(txns) != 1 {
			t.Fatal("wrong number of transactions")
		}
		if txn.ID() != txns[0] {
			t.Fatal("wrong transaction")
		}
		txns0, err := e.Transaction(txns[0])
		if err != nil {
			t.Fatal(err)
		}
		if txn.ID() != txns0.ID() {
			t.Fatal("wrong transaction")
		}
	}
}

func TestChainStatsSiacoins(t *testing.T) {
	sim := chainutil.NewChainSim()
	cm := chain.NewManager(chainutil.NewEphemeralStore(sim.Genesis), sim.State)

	hs, err := explorerutil.NewHashStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	explorerStore := explorerutil.NewEphemeralStore()
	e := explorer.NewExplorer(sim.Genesis.State, explorerStore, hs)
	cm.AddSubscriber(e, cm.Tip())
	if err := addGenesisElements(e, sim.Genesis.Block); err != nil {
		t.Fatal(err)
	}

	w := walletutil.NewTestingWallet(cm.TipState())
	cm.AddSubscriber(w, cm.Tip())

	// fund the wallet with 100 coins
	ourAddr := w.NewAddress()
	fund := types.SiacoinOutput{Value: types.Siacoins(100), Address: ourAddr}
	if err := cm.AddTipBlock(sim.MineBlockWithSiacoinOutputs(fund)); err != nil {
		t.Fatal(err)
	}

	// empty block with nothing beside miner reward
	if err := cm.AddTipBlock(sim.MineBlockWithTxns()); err != nil {
		t.Fatal(err)
	}
	stats, err := e.ChainStatsLatest()
	if err != nil {
		t.Fatal(err)
	}
	expected := explorer.ChainStats{
		// don't compare these
		Block: stats.Block,

		SpentSiacoinsCount:  0,
		SpentSiafundsCount:  0,
		ActiveContractCost:  types.Siacoins(825),
		ActiveContractCount: 10,
		ActiveContractSize:  0,
		TotalContractCost:   types.Siacoins(825),
		TotalContractSize:   0,
		TotalRevisionVolume: 0,
	}
	if !reflect.DeepEqual(stats, expected) {
		t.Fatal("chainstats don't match")
	}

	size, err := e.Size()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Begin size: %d bytes", size)

	n := 1000
	for i := 0; i < n; i++ {
		sendAmount := types.Siacoins(7).Div64(1000)
		txn := types.Transaction{
			SiacoinOutputs: []types.SiacoinOutput{{
				Address: types.VoidAddress,
				Value:   sendAmount,
			}},
		}
		if err := w.FundAndSign(&txn); err != nil {
			t.Fatal(err)
		}

		if err := cm.AddTipBlock(sim.MineBlockWithTxns(txn)); err != nil {
			t.Fatal(err)
		}

		stats, err := e.ChainStatsLatest()
		if err != nil {
			t.Fatal(err)
		}
		expected := explorer.ChainStats{
			// don't compare these
			Block: stats.Block,

			SpentSiacoinsCount:  1,
			SpentSiafundsCount:  0,
			ActiveContractCost:  types.Siacoins(825),
			ActiveContractCount: 10,
			ActiveContractSize:  0,
			TotalContractCost:   types.Siacoins(825),
			TotalContractSize:   0,
			TotalRevisionVolume: 0,
		}
		if !reflect.DeepEqual(stats, expected) {
			t.Fatal("chainstats don't match")
		}
	}

	size, err = e.Size()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("End size (after %d blocks): %d bytes (average %d bytes/block)", n, size, size/uint64(n))
}

func TestChainStatsContracts(t *testing.T) {
	sim := chainutil.NewChainSim()
	cm := chain.NewManager(chainutil.NewEphemeralStore(sim.Genesis), sim.State)

	hs, err := explorerutil.NewHashStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	explorerStore := explorerutil.NewEphemeralStore()
	e := explorer.NewExplorer(sim.Genesis.State, explorerStore, hs)
	cm.AddSubscriber(e, cm.Tip())
	if err := addGenesisElements(e, sim.Genesis.Block); err != nil {
		t.Fatal(err)
	}

	w := walletutil.NewTestingWallet(cm.TipState())
	cm.AddSubscriber(w, cm.Tip())

	ourAddr := w.NewAddress()
	renterPubkey, renterPrivkey := testingKeypair(1)
	hostPubkey, hostPrivkey := testingKeypair(2)
	if err := cm.AddTipBlock(sim.MineBlockWithSiacoinOutputs(
		types.SiacoinOutput{Value: types.Siacoins(100), Address: ourAddr},
		types.SiacoinOutput{Value: types.Siacoins(100), Address: types.StandardAddress(renterPubkey)},
		types.SiacoinOutput{Value: types.Siacoins(7), Address: types.StandardAddress(hostPubkey)},
	)); err != nil {
		t.Fatal(err)
	}

	renterOutputs, err := e.UnspentSiacoinElements(types.StandardAddress(renterPubkey))
	if err != nil {
		t.Fatal(err)
	}
	renterOutput, err := e.SiacoinElement(renterOutputs[0])
	if err != nil {
		t.Fatal(err)
	}

	hostOutputs, err := e.UnspentSiacoinElements(types.StandardAddress(hostPubkey))
	if err != nil {
		t.Fatal(err)
	}
	hostOutput, err := e.SiacoinElement(hostOutputs[0])
	if err != nil {
		t.Fatal(err)
	}

	// form initial contract
	initialRev := types.FileContract{
		WindowStart: 5,
		WindowEnd:   10,
		RenterOutput: types.SiacoinOutput{
			Address: types.StandardAddress(renterPubkey),
			Value:   types.Siacoins(58),
		},
		HostOutput: types.SiacoinOutput{
			Address: types.StandardAddress(hostPubkey),
			Value:   types.Siacoins(19),
		},
		MissedHostValue: types.Siacoins(17),
		TotalCollateral: types.Siacoins(18),
		RenterPublicKey: renterPubkey,
		HostPublicKey:   hostPubkey,
	}
	outputSum := initialRev.RenterOutput.Value.Add(initialRev.HostOutput.Value).Add(cm.TipState().FileContractTax(initialRev))

	if renterOutput.MerkleProof, err = e.MerkleProof(renterOutput.ID); err != nil {
		t.Fatal(err)
	}
	if hostOutput.MerkleProof, err = e.MerkleProof(hostOutput.ID); err != nil {
		t.Fatal(err)
	}
	txn := types.Transaction{
		SiacoinInputs: []types.SiacoinInput{
			{Parent: renterOutput, SpendPolicy: types.PolicyPublicKey(renterPubkey)},
			{Parent: hostOutput, SpendPolicy: types.PolicyPublicKey(hostPubkey)},
		},
		FileContracts: []types.FileContract{initialRev},
		MinerFee:      renterOutput.Value.Add(hostOutput.Value).Sub(outputSum),
	}
	fc := &txn.FileContracts[0]
	contractHash := cm.TipState().ContractSigHash(*fc)
	fc.RenterSignature = renterPrivkey.SignHash(contractHash)
	fc.HostSignature = hostPrivkey.SignHash(contractHash)
	sigHash := cm.TipState().InputSigHash(txn)
	txn.SiacoinInputs[0].Signatures = []types.Signature{renterPrivkey.SignHash(sigHash)}
	txn.SiacoinInputs[1].Signatures = []types.Signature{hostPrivkey.SignHash(sigHash)}

	if err := cm.AddTipBlock(sim.MineBlockWithTxns(txn)); err != nil {
		t.Fatal(err)
	}
	stats, err := e.ChainStatsLatest()
	if err != nil {
		t.Fatal(err)
	}
	expected := explorer.ChainStats{
		// don't compare these
		Block: stats.Block,

		SpentSiacoinsCount:  2,
		SpentSiafundsCount:  0,
		ActiveContractCost:  types.Siacoins(825).Add(types.Siacoins(77)),
		ActiveContractCount: 10 + 1,
		ActiveContractSize:  0,
		TotalContractCost:   types.Siacoins(825).Add(types.Siacoins(77)),
		TotalContractSize:   0,
		TotalRevisionVolume: 0,
	}
	if !reflect.DeepEqual(stats, expected) {
		t.Fatal("chainstats don't match")
	}
}

func BenchmarkAddEmptyBlocks(b *testing.B) {
	b.StopTimer()

	sim := chainutil.NewChainSim()
	cm := chain.NewManager(chainutil.NewEphemeralStore(sim.Genesis), sim.State)

	hs, err := explorerutil.NewHashStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	explorerStore := explorerutil.NewEphemeralStore()
	e := explorer.NewExplorer(sim.Genesis.State, explorerStore, hs)
	if err := addGenesisElements(e, sim.Genesis.Block); err != nil {
		b.Fatal(err)
	}
	cm.AddSubscriber(e, cm.Tip())
	b.Log(b.N)
	blocks := sim.MineBlocks(b.N)

	b.StartTimer()
	cm.AddBlocks(blocks)
}

func BenchmarkSiacoinElement(b *testing.B) {
	b.StopTimer()

	sim := chainutil.NewChainSim()
	cm := chain.NewManager(chainutil.NewEphemeralStore(sim.Genesis), sim.State)

	hs, err := explorerutil.NewHashStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	explorerStore := explorerutil.NewEphemeralStore()
	e := explorer.NewExplorer(sim.Genesis.State, explorerStore, hs)
	if err := addGenesisElements(e, sim.Genesis.Block); err != nil {
		b.Fatal(err)
	}
	cm.AddSubscriber(e, cm.Tip())
	au := consensus.GenesisUpdate(sim.Genesis.Block, types.Work{NumHashes: [32]byte{31: 4}})

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		id := au.NewSiacoinElements[i%10].ID

		elem, err := e.SiacoinElement(id)
		if err != nil {
			b.Fatal(err)
		}
		if elem.ID != id {
			b.Fatal("wrong element")
		}
	}
}

func BenchmarkMerkleProof(b *testing.B) {
	b.StopTimer()

	sim := chainutil.NewChainSim()
	cm := chain.NewManager(chainutil.NewEphemeralStore(sim.Genesis), sim.State)

	hs, err := explorerutil.NewHashStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	explorerStore := explorerutil.NewEphemeralStore()
	e := explorer.NewExplorer(sim.Genesis.State, explorerStore, hs)
	if err := addGenesisElements(e, sim.Genesis.Block); err != nil {
		b.Fatal(err)
	}
	cm.AddSubscriber(e, cm.Tip())
	au := consensus.GenesisUpdate(sim.Genesis.Block, types.Work{NumHashes: [32]byte{31: 4}})
	cm.AddBlocks(sim.MineBlocks(1000))
	cs := cm.TipState()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		elem := au.NewSiacoinElements[i%10]
		if elem.MerkleProof, err = e.MerkleProof(elem.ID); err != nil {
			b.Fatal(err)
		}

		if !cs.Elements.ContainsUnspentSiacoinElement(elem) {
			b.Fatal("accumulator should have unspent genesis output")
		} else if cs.Elements.ContainsSpentSiacoinElement(elem) {
			b.Fatal("accumulator should not see genesis output as spent")
		}
	}
}
