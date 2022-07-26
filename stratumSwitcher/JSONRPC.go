package main

import (
	"encoding/json"
)

// JSONRPCRequest JSON RPC Requested data structure
type JSONRPCRequest struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`

	// Worker: ETHProxy from ethminer may contains this field
	Worker string `json:"worker,omitempty"`
}

// JSONRPCResponse JSON RPC response data structure
type JSONRPCResponse struct {
	ID     interface{} `json:"id"`
	Result interface{} `json:"result"`
	Error  interface{} `json:"error"`
}

// JSONRPC2Error error object of json-rpc 2.0
type JSONRPC2Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// NewJSONRPC2Error create json-rpc 2.0 error object from json-1.0 error object
func NewJSONRPC2Error(v1Err interface{}) (err *JSONRPC2Error) {
	if v1Err == nil {
		return nil
	}

	errArr, ok := v1Err.(JSONRPCArray)
	if !ok {
		return nil
	}

	err = new(JSONRPC2Error)
	if len(errArr) >= 1 {
		code, ok := errArr[0].(int)
		if ok {
			err.Code = code
		}
	}
	if len(errArr) >= 2 {
		message, ok := errArr[1].(string)
		if ok {
			err.Message = message
		}
	}
	if len(errArr) >= 3 {
		err.Data = errArr[2]
	}
	return
}

// JSONRPC2Response response message of json-rpc 2.0
type JSONRPC2Response struct {
	ID      interface{}    `json:"id"`
	JSONRPC string         `json:"jsonrpc"`
	Result  interface{}    `json:"result,omitempty"`
	Error   *JSONRPC2Error `json:"error,omitempty"`
}

// JSONRPCArray JSON RPC array
type JSONRPCArray []interface{}

// JSONRPCObj JSON RPC object
type JSONRPCObj map[string]interface{}

// NewJSONRPCRequest Parse JSON RPC request string and create JSONRPCRequest object
func NewJSONRPCRequest(rpcJSON []byte) (*JSONRPCRequest, error) {
	rpcData := new(JSONRPCRequest)

	err := json.Unmarshal(rpcJSON, &rpcData)

	return rpcData, err
}

// AddParam 向 JSONRPCRequest object adds one or more parameters
func (rpcData *JSONRPCRequest) AddParam(param ...interface{}) {
	rpcData.Params = append(rpcData.Params, param...)
}

// SetParam Set the parameters of the JSONRPCRequest object
// The list of parameters passed to SetParam will be copied into JSONRPCRequest.Params in order
func (rpcData *JSONRPCRequest) SetParam(param ...interface{}) {
	rpcData.Params = param
}

// ToJSONBytes Convert JSONRPCRequest object to JSON byte sequence
func (rpcData *JSONRPCRequest) ToJSONBytes() ([]byte, error) {
	return json.Marshal(rpcData)
}

// NewJSONRPCResponse Parse JSON RPC response string and create JSONRPCResponse object
func NewJSONRPCResponse(rpcJSON []byte) (*JSONRPCResponse, error) {
	rpcData := new(JSONRPCResponse)

	err := json.Unmarshal(rpcJSON, &rpcData)

	return rpcData, err
}

// SetResult Set the return result of the JSONRPCResponse object
func (rpcData *JSONRPCResponse) SetResult(result interface{}) {
	rpcData.Result = result
}

// ToJSONBytesConvert a JSONRPCResponse object to a JSON byte sequence
func (rpcData *JSONRPCResponse) ToJSONBytes(version int) ([]byte, error) {
	if version == 1 {
		return json.Marshal(rpcData)
	}

	rpc2Data := JSONRPC2Response{rpcData.ID, "2.0", rpcData.Result, NewJSONRPC2Error(rpcData.Error)}
	return json.Marshal(rpc2Data)
}
