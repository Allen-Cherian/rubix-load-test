package rubix

// Wire types for the rubix node HTTP API. These mirror the JSON shapes used
// by POST /rubix/v1/tx and POST /rubix/v1/signature so this project has no
// dependency on the rubixgoplatform Go module.

// TransactionRequest is the body for POST /rubix/v1/tx.
type TransactionRequest struct {
	Initiator string                  `json:"initiator"`
	Owner     string                  `json:"owner"`
	Tokens    TransactionTokenDetails `json:"tokens"`
	Memo      string                  `json:"memo"`
}

type TransactionTokenDetails struct {
	RBT                  float64       `json:"rbt"`
	FT                   []interface{} `json:"ft"`
	NFT                  []interface{} `json:"nft"`
	SmartContract        []interface{} `json:"smartContract"`
	TransferNFTOwnership bool          `json:"transferNftOwnership"`
}

// BasicResponse is the generic response envelope used across most endpoints.
type BasicResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result"`
}

// RBTBalance is the Result payload returned by GET /rubix/v1/dids/{did}/balances/rbt.
type RBTBalance struct {
	Balance float64 `json:"balance"`
	Pledged float64 `json:"pledged"`
	Locked  float64 `json:"locked"`
}

// SignReqData is the challenge payload returned inside BasicResponse.Result
// when the node needs the initiator to complete a signature step.
type SignReqData struct {
	ID   string `json:"id"`
	Hash []byte `json:"hash"`
}

// SignRespData is the body for POST /rubix/v1/signature.
type SignRespData struct {
	ID        string `json:"id"`
	Password  string `json:"password"`
	Signature string `json:"signature"` // base64 — empty when the node signs with the unlocked DID
}

// Endpoint paths.
const (
	EndpointTransaction = "/rubix/v1/tx"
	EndpointSignature   = "/rubix/v1/signature"
	EndpointRBTBalance  = "/rubix/v1/dids/{did}/balances/rbt"
)
