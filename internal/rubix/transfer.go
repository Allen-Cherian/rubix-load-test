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
// A transfer is only considered successful when the signature-response step
// returns status=true AND a non-empty transactionID in its result.
type TransferResult struct {
	TransactionID string
	Status        bool
	Message       string
}

// Transfer performs the full two-step flow: POST /rubix/v1/tx, then
// POST /rubix/v1/signature. Success requires a transactionID in the final
// response; anything else is treated as failure.
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
		return TransferResult{Status: false, Message: "tx: " + err.Error()}
	}
	if !br.Status {
		return TransferResult{Status: false, Message: "tx: " + br.Message}
	}
	if br.Result == nil {
		return TransferResult{Status: false, Message: "tx: no signature challenge returned"}
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
		return TransferResult{Status: false, Message: "sig: " + err.Error()}
	}
	if !br2.Status {
		return TransferResult{Status: false, Message: "sig: " + br2.Message}
	}
	if br2.Result == nil {
		return TransferResult{Status: false, Message: "sig: status=true but result is null"}
	}

	sb, err := json.Marshal(br2.Result)
	if err != nil {
		return TransferResult{Status: false, Message: "marshal sig result: " + err.Error()}
	}
	var ts TransferSuccess
	if err := json.Unmarshal(sb, &ts); err != nil {
		return TransferResult{Status: false, Message: "decode sig result: " + err.Error()}
	}
	if ts.TransactionID == "" {
		return TransferResult{Status: false, Message: "sig: empty transactionID in result"}
	}
	return TransferResult{TransactionID: ts.TransactionID, Status: true, Message: br2.Message}
}
