package rubix

import (
	"encoding/json"
	"fmt"
)

// InitiateTransfer posts a TransactionRequest to /rubix/v1/tx and returns the
// raw BasicResponse from the node. If the node requires a signature challenge,
// the caller should follow up with SignatureResponse using the ID in the result.
func (c *Client) InitiateTransfer(req *TransactionRequest) (*BasicResponse, error) {
	var resp BasicResponse
	if err := c.doJSON("POST", EndpointTransaction, nil, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SignatureResponse completes the signature challenge step.
func (c *Client) SignatureResponse(sr *SignRespData) (*BasicResponse, error) {
	var resp BasicResponse
	if err := c.doJSON("POST", EndpointSignature, nil, sr, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetRBTBalance hits /rubix/v1/dids/{did}/balances/rbt and returns the typed
// balance breakdown along with the raw envelope (for message/status).
func (c *Client) GetRBTBalance(did string) (*RBTBalance, *BasicResponse, error) {
	var resp BasicResponse
	if err := c.doJSON("GET", EndpointRBTBalance, map[string]string{"did": did}, nil, &resp); err != nil {
		return nil, nil, err
	}
	if !resp.Status || resp.Result == nil {
		return nil, &resp, nil
	}
	jb, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, &resp, fmt.Errorf("marshal balance result: %w", err)
	}
	var bal RBTBalance
	if err := json.Unmarshal(jb, &bal); err != nil {
		return nil, &resp, fmt.Errorf("decode balance result: %w", err)
	}
	return &bal, &resp, nil
}

// TransferResult is the outcome of a single end-to-end transfer attempt.
type TransferResult struct {
	ReqID   string
	Status  bool
	Message string
}

// Transfer performs the full two-step flow: POST /rubix/v1/tx, and if the node
// returns a signature challenge, follows up with POST /rubix/v1/signature.
func (c *Client) Transfer(initiator, owner string, amount float64, password, memo string) TransferResult {
	req := &TransactionRequest{
		Initiator: initiator,
		Owner:     owner,
		Tokens: TransactionTokenDetails{
			RBT:                  amount,
			FT:                   []interface{}{},
			NFT:                  []interface{}{},
			SmartContract:        []interface{}{},
			TransferNFTOwnership: false,
		},
		Memo: memo,
	}

	br, err := c.InitiateTransfer(req)
	if err != nil {
		return TransferResult{Status: false, Message: err.Error()}
	}
	if !br.Status {
		return TransferResult{Status: false, Message: br.Message}
	}
	if br.Result == nil {
		return TransferResult{Status: true, Message: br.Message}
	}

	// Signature challenge — result is a SignReqData.
	jb, err := json.Marshal(br.Result)
	if err != nil {
		return TransferResult{Status: false, Message: "marshal sign challenge: " + err.Error()}
	}
	var sr SignReqData
	if err := json.Unmarshal(jb, &sr); err != nil {
		return TransferResult{Status: false, Message: "decode sign challenge: " + err.Error()}
	}

	sresp := &SignRespData{
		ID:       sr.ID,
		Password: password,
	}
	br2, err := c.SignatureResponse(sresp)
	if err != nil {
		return TransferResult{ReqID: sr.ID, Status: false, Message: err.Error()}
	}
	if !br2.Status {
		return TransferResult{ReqID: sr.ID, Status: false, Message: br2.Message}
	}
	return TransferResult{ReqID: sr.ID, Status: true, Message: fmt.Sprintf("%v", br2.Message)}
}
