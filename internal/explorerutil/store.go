package explorerutil

import (
	"bytes"
	"context"
	"database/sql"
	"strings"

	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/explorer"
	_ "modernc.org/sqlite"
)

func encode(obj types.EncoderTo) []byte {
	var buf bytes.Buffer
	e := types.NewEncoder(&buf)
	obj.EncodeTo(e)
	e.Flush()
	return buf.Bytes()
}

func decode(obj types.DecoderFrom, data []byte) error {
	d := types.NewBufDecoder(data)
	obj.DecodeFrom(d)
	return d.Err()
}

func scan(rows *sql.Rows, d types.DecoderFrom) error {
	var data []byte
	if err := rows.Scan(&data); err != nil {
		return err
	}
	return decode(d, data)
}

// SQLiteStore implements explorer.Store using a SQLite database.
type SQLiteStore struct {
	db    *sql.DB
	tx    *sql.Tx
	txErr error
}

func (s *SQLiteStore) beginTx() {
	if s.tx == nil {
		s.tx, s.txErr = s.db.BeginTx(context.Background(), nil)
	}
}

func (s *SQLiteStore) query(query string, args ...interface{}) (*sql.Rows, error) {
	s.beginTx()
	if s.txErr != nil {
		return nil, s.txErr
	}
	return s.tx.Query(query, args...)
}

func (s *SQLiteStore) queryRow(d types.DecoderFrom, query string, args ...interface{}) error {
	s.beginTx()
	if s.txErr != nil {
		return s.txErr
	}
	var data []byte
	s.txErr = s.tx.QueryRow(query, args...).Scan(&data)
	if s.txErr == nil {
		s.txErr = decode(d, data)
	}
	return s.txErr
}

func (s *SQLiteStore) execStatement(statement string, args ...interface{}) {
	s.beginTx()
	if s.txErr != nil {
		return
	}
	if stmt, err := s.tx.Prepare(statement); err != nil {
		s.txErr = err
	} else if _, err := stmt.Exec(args...); err != nil {
		s.txErr = err
		stmt.Close()
	} else {
		s.txErr = stmt.Close()
	}
}

// Commit implements explorer.Store.
func (s *SQLiteStore) Commit() (err error) {
	if s.txErr != nil {
		s.tx.Rollback() // TODO: return this error?
		err = s.txErr
	} else {
		err = s.tx.Commit()
	}
	s.tx = nil
	return
}

// SiacoinElement implements explorer.Store.
func (s *SQLiteStore) SiacoinElement(id types.ElementID) (sce types.SiacoinElement, err error) {
	err = s.queryRow(&sce, `SELECT data FROM elements WHERE id=? AND type=?`, encode(id), "siacoin")
	return
}

// SiafundElement implements explorer.Store.
func (s *SQLiteStore) SiafundElement(id types.ElementID) (sfe types.SiafundElement, err error) {
	err = s.queryRow(&sfe, `SELECT data FROM elements WHERE id=? AND type=?`, encode(id), "siafund")
	return
}

// FileContractElement implements explorer.Store.
func (s *SQLiteStore) FileContractElement(id types.ElementID) (fce types.FileContractElement, err error) {
	err = s.queryRow(&fce, `SELECT data FROM elements WHERE id=? AND type=?`, encode(id), "contract")
	return
}

// ChainStats implements explorer.Store.
func (s *SQLiteStore) ChainStats(index types.ChainIndex) (cs explorer.ChainStats, err error) {
	err = s.queryRow(&cs, `SELECT data FROM chainstats WHERE id=?`, index.String())
	return
}

// UnspentSiacoinElements implements explorer.Store.
func (s *SQLiteStore) UnspentSiacoinElements(address types.Address) ([]types.ElementID, error) {
	rows, err := s.query(`SELECT id FROM unspentElements WHERE address=? AND type=?`, encode(address), "siacoin")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []types.ElementID
	for rows.Next() {
		var id types.ElementID
		if err := scan(rows, &id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UnspentSiafundElements implements explorer.Store.
func (s *SQLiteStore) UnspentSiafundElements(address types.Address) ([]types.ElementID, error) {
	rows, err := s.query(`SELECT id FROM unspentElements WHERE address=? AND type=?`, encode(address), "siafund")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []types.ElementID
	for rows.Next() {
		var id types.ElementID
		if err := scan(rows, &id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Transaction implements explorer.Store.
func (s *SQLiteStore) Transaction(id types.TransactionID) (txn types.Transaction, err error) {
	err = s.queryRow(&txn, `SELECT data FROM transactions WHERE id=?`, encode(id))
	return
}

// Transactions implements explorer.Store.
func (s *SQLiteStore) Transactions(address types.Address, amount, offset int) ([]types.TransactionID, error) {
	rows, err := s.query(`SELECT id FROM addressTransactions WHERE address=? LIMIT ? OFFSET ?`, encode(address), amount, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []types.TransactionID
	for rows.Next() {
		var id types.TransactionID
		if err := scan(rows, &id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// State implements explorer.Store.
func (s *SQLiteStore) State(index types.ChainIndex) (context consensus.State, err error) {
	err = s.queryRow(&context, `SELECT data FROM states WHERE id=?`, encode(index))
	return
}

// AddSiacoinElement implements explorer.Store.
func (s *SQLiteStore) AddSiacoinElement(sce types.SiacoinElement) {
	s.execStatement(`INSERT INTO elements(id, type, data) VALUES(?, ?, ?)`, encode(sce.ID), "siacoin", encode(sce))
}

// AddSiafundElement implements explorer.Store.
func (s *SQLiteStore) AddSiafundElement(sfe types.SiafundElement) {
	s.execStatement(`INSERT INTO elements(id, type, data) VALUES(?, ?, ?)`, encode(sfe.ID), "siafund", encode(sfe))
}

// AddFileContractElement implements explorer.Store.
func (s *SQLiteStore) AddFileContractElement(fce types.FileContractElement) {
	s.execStatement(`INSERT INTO elements(id, type, data) VALUES(?, ?, ?)`, encode(fce.ID), "contract", encode(fce))
}

// RemoveElement implements explorer.Store.
func (s *SQLiteStore) RemoveElement(id types.ElementID) {
	s.execStatement(`DELETE FROM elements WHERE id=?`, encode(id))
}

// AddChainStats implements explorer.Store.
func (s *SQLiteStore) AddChainStats(index types.ChainIndex, cs explorer.ChainStats) {
	s.execStatement(`INSERT INTO chainstats(id, data) VALUES(?, ?)`, index.String(), encode(cs))
}

// AddUnspentSiacoinElement implements explorer.Store.
func (s *SQLiteStore) AddUnspentSiacoinElement(address types.Address, id types.ElementID) {
	s.execStatement(`INSERT INTO unspentElements(address, type, id) VALUES(?, ?, ?)`, encode(address), "siacoin", encode(id))
}

// AddUnspentSiafundElement implements explorer.Store.
func (s *SQLiteStore) AddUnspentSiafundElement(address types.Address, id types.ElementID) {
	s.execStatement(`INSERT INTO unspentElements(address, type, id) VALUES(?, ?, ?)`, encode(address), "siafund", encode(id))
}

// RemoveUnspentSiacoinElement implements explorer.Store.
func (s *SQLiteStore) RemoveUnspentSiacoinElement(address types.Address, id types.ElementID) {
	s.execStatement(`DELETE FROM unspentElements WHERE address=? AND id=? AND type=?`, encode(address), encode(id), "siacoin")
}

// RemoveUnspentSiafundElement implements explorer.Store.
func (s *SQLiteStore) RemoveUnspentSiafundElement(address types.Address, id types.ElementID) {
	s.execStatement(`DELETE FROM unspentElements WHERE address=? AND id=? AND type=?`, encode(address), encode(id), "siafund")
}

// AddTransaction implements explorer.Store.
func (s *SQLiteStore) AddTransaction(txn types.Transaction, addresses []types.Address, block types.ChainIndex) {
	id := encode(txn.ID())
	s.execStatement(`INSERT INTO transactions(id, data) VALUES(?, ?)`, id, encode(txn))

	for _, address := range addresses {
		s.execStatement(`INSERT INTO addressTransactions(address, id) VALUES(?, ?)`, encode(address), id)
	}
}

// AddState implements explorer.Store.
func (s *SQLiteStore) AddState(index types.ChainIndex, context consensus.State) {
	s.execStatement(`INSERT INTO states(id, data) VALUES(?, ?)`, encode(index), encode(context))
}

func createTables(db *sql.DB) error {
	query := `
CREATE TABLE elements (
	id BINARY(128) PRIMARY KEY,
	type BINARY(128),
	data BLOB NOT NULL
);

CREATE TABLE states (
	id BINARY(128) PRIMARY KEY,
	data BLOB NOT NULL
);

CREATE TABLE chainstats (
	id BINARY(128) PRIMARY KEY,
	data BLOB NOT NULL
);

CREATE TABLE unspentElements (
	id BINARY(128) PRIMARY KEY,
	type BINARY(128),
	address BINARY(128)
);

CREATE TABLE transactions (
	id BINARY(128) PRIMARY KEY,
	data BLOB NOT NULL
);

CREATE TABLE addressTransactions (
	id BINARY(128),
	address BINARY(128)
);
`
	_, err := db.Exec(query)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		err = nil
	}
	return err
}

// NewStore creates a new SQLiteStore for storing explorer data.
func NewStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	} else if err := createTables(db); err != nil {
		return nil, err
	}
	return &SQLiteStore{db: db}, nil
}

// NewEphemeralStore returns a new in-memory SQLiteStore.
func NewEphemeralStore() *SQLiteStore {
	s, err := NewStore(":memory:")
	if err != nil {
		panic(err)
	}
	return s
}
