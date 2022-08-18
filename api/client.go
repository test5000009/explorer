package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
	"go.sia.tech/explorer"
)

// A Client provides methods for interacting with a explorerd API server.
type Client struct {
	BaseURL      string
	AuthPassword string
}

func (c *Client) req(method string, route string, data, resp interface{}) error {
	var body io.Reader
	if data != nil {
		js, _ := json.Marshal(data)
		body = bytes.NewReader(js)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("%v%v", c.BaseURL, route), body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("", c.AuthPassword)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer io.Copy(ioutil.Discard, r.Body)
	defer r.Body.Close()
	if r.StatusCode != 200 {
		err, _ := ioutil.ReadAll(r.Body)
		return errors.New(string(err))
	}
	if resp == nil {
		return nil
	}
	return json.NewDecoder(r.Body).Decode(resp)
}

func (c *Client) get(route string, r interface{}) error     { return c.req("GET", route, nil, r) }
func (c *Client) post(route string, d, r interface{}) error { return c.req("POST", route, d, r) }
func (c *Client) put(route string, d interface{}) error     { return c.req("PUT", route, d, nil) }
func (c *Client) delete(route string) error                 { return c.req("DELETE", route, nil, nil) }

// WriteJSON writes the JSON encoded object to the http response.
func WriteJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

// TxpoolBroadcast broadcasts a transaction to the network.
func (c *Client) TxpoolBroadcast(txn types.Transaction, dependsOn []types.Transaction) (err error) {
	err = c.post("/api/txpool/broadcast", TxpoolBroadcastRequest{dependsOn, txn}, nil)
	return
}

// TxpoolTransactions returns all transactions in the transaction pool.
func (c *Client) TxpoolTransactions() (resp []types.Transaction, err error) {
	err = c.get("/api/txpool/transactions", &resp)
	return
}

// SyncerPeers returns the current peers of the syncer.
func (c *Client) SyncerPeers() (resp []SyncerPeerResponse, err error) {
	err = c.get("/api/syncer/peers", &resp)
	return
}

// SyncerConnect adds the address as a peer of the syncer.
func (c *Client) SyncerConnect(addr string) (err error) {
	err = c.post("/api/syncer/connect", addr, nil)
	return
}

// ChainStats returns stats about the chain at the given index.
func (c *Client) ChainStats(index types.ChainIndex) (resp explorer.ChainStats, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/chain/%s", index.String()), &resp)
	return
}

// ChainState returns the validation context at a given chain index.
func (c *Client) ChainState(index types.ChainIndex) (resp consensus.State, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/chain/%s/state", index.String()), &resp)
	return
}

// SiacoinElement returns the Siacoin element with the given ID.
func (c *Client) SiacoinElement(id types.ElementID) (resp types.SiacoinElement, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/element/siacoin/%s", id.String()), &resp)
	return
}

// SiafundElement returns the Siafund element with the given ID.
func (c *Client) SiafundElement(id types.ElementID) (resp types.SiafundElement, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/element/siafund/%s", id.String()), &resp)
	return
}

// FileContractElement returns the file contract element with the given ID.
func (c *Client) FileContractElement(id types.ElementID) (resp types.FileContractElement, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/element/contract/%s", id.String()), &resp)
	return
}

// ElementSearch returns information about a given element.
func (c *Client) ElementSearch(id types.ElementID) (resp ExplorerSearchResponse, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/element/search/%s", id.String()), &resp)
	return
}

// AddressBalance returns the siacoin and siafund balance of an address.
func (c *Client) AddressBalance(address types.Address) (resp ExplorerWalletBalanceResponse, err error) {
	data, err := json.Marshal(address)
	if err != nil {
		return
	}
	err = c.get(fmt.Sprintf("/api/explorer/address/%s/balance", string(data)), &resp)
	return
}

// SiacoinOutputs returns the unspent siacoin elements of an address.
func (c *Client) SiacoinOutputs(address types.Address) (resp []types.ElementID, err error) {
	data, err := json.Marshal(address)
	if err != nil {
		return
	}
	err = c.get(fmt.Sprintf("/api/explorer/address/%s/siacoins", string(data)), &resp)
	return
}

// SiafundOutputs returns the unspent siafunds elements of an address.
func (c *Client) SiafundOutputs(address types.Address) (resp []types.ElementID, err error) {
	data, err := json.Marshal(address)
	if err != nil {
		return
	}
	err = c.get(fmt.Sprintf("/api/explorer/address/%s/siafunds", string(data)), &resp)
	return
}

// Transactions returns the latest transaction IDs the address was involved in.
func (c *Client) Transactions(address types.Address, amount, offset int) (resp []types.TransactionID, err error) {
	data, err := json.Marshal(address)
	if err != nil {
		return
	}
	err = c.get(fmt.Sprintf("/api/explorer/address/%s/transactions?amount=%d&offset=%d", string(data), amount, offset), &resp)
	return
}

// Transaction returns a transaction with the given ID.
func (c *Client) Transaction(id types.TransactionID) (resp types.Transaction, err error) {
	err = c.get(fmt.Sprintf("/api/explorer/transaction/%s", id.String()), &resp)
	return
}

// BatchBalance returns the siacoin and siafund balance of a list of addresses.
func (c *Client) BatchBalance(addresses []types.Address) (resp []ExplorerWalletBalanceResponse, err error) {
	err = c.post("/api/explorer/batch/addresses/balance", addresses, &resp)
	return
}

// BatchSiacoins returns the unspent siacoin elements of the addresses.
func (c *Client) BatchSiacoins(addresses []types.Address) (resp [][]types.SiacoinElement, err error) {
	err = c.post("/api/explorer/batch/addresses/siacoins", addresses, &resp)
	return
}

// BatchSiafunds returns the unspent siafund elements of the addresses.
func (c *Client) BatchSiafunds(addresses []types.Address) (resp [][]types.SiafundElement, err error) {
	err = c.post("/api/explorer/batch/addresses/siafunds", addresses, &resp)
	return
}

// BatchTransactions returns the last n transactions of the addresses.
func (c *Client) BatchTransactions(addresses []ExplorerTransactionsRequest) (resp [][]types.Transaction, err error) {
	err = c.post("/api/explorer/batch/addresses/transactions", addresses, &resp)
	return
}

// NewClient returns a client that communicates with a explorerd server
// listening on the specified address.
func NewClient(addr, password string) *Client {
	return &Client{
		BaseURL:      addr,
		AuthPassword: password,
	}
}
