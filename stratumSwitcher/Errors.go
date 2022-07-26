package main

import "errors"

// StratumError Stratum error
type StratumError struct {
	// error number
	ErrNo int
	// error message
	ErrMsg string
}

// NewStratumError Create a new StratumError
func NewStratumError(errNo int, errMsg string) *StratumError {
	err := new(StratumError)
	err.ErrNo = errNo
	err.ErrMsg = errMsg

	return err
}

// Error Implements the Error() interface of StratumError so that it can be used as an error type
func (err *StratumError) Error() string {
	return err.ErrMsg
}

// ToJSONRPCArray Convert to JSONRPCArray
func (err *StratumError) ToJSONRPCArray(extData interface{}) JSONRPCArray {
	if err == nil {
		return nil
	}

	return JSONRPCArray{err.ErrNo, err.ErrMsg, extData}
}

var (
	// ErrBufIOReadTimeout timed out reading data from bufio.Reader
	ErrBufIOReadTimeout = errors.New("BufIO Read Timeout")
	// ErrSessionIDFull SessionID is full (all available values ​​are assigned)
	ErrSessionIDFull = errors.New("Session ID is Full")
	// ErrSessionIDOccupied SessionID has been occupied (when restoring SessionID)
	ErrSessionIDOccupied = errors.New("Session ID has been occupied")
	// ErrParseSubscribeResponseFailed Failed to parse subscription response
	ErrParseSubscribeResponseFailed = errors.New("Parse Subscribe Response Failed")
	// The session ID returned by ErrSessionIDInconformity does not match the currently saved session ID
	ErrSessionIDInconformity = errors.New("Session ID Inconformity")
	// ErrAuthorizeFailed Authentication failed
	ErrAuthorizeFailed = errors.New("Authorize Failed")
	// ErrTooMuchPendingAutoRegReq Too many pending auto-registration requests
	ErrTooMuchPendingAutoRegReq = errors.New("Too much pending auto reg request")
)

var (
	// StratumErrNeedSubscribed requires subscription
	StratumErrNeedSubscribed = NewStratumError(101, "Need Subscribed")
	// StratumErrDuplicateSubscribed Duplicate Subscribed
	StratumErrDuplicateSubscribed = NewStratumError(102, "Duplicate Subscribed")
	// StratumErrTooFewParams too few parameters
	StratumErrTooFewParams = NewStratumError(103, "Too Few Params")
	// StratumErrWorkerNameMustBeString miner name must be a string
	StratumErrWorkerNameMustBeString = NewStratumError(104, "Worker Name Must be a String")
	// StratumErrWorkerNameStartWrong Miner name starts incorrectly
	StratumErrWorkerNameStartWrong = NewStratumError(105, "Sub-account Name Cannot be Empty")

	// StratumErrStratumServerNotFound The Stratum Server of the corresponding currency could not be found
	StratumErrStratumServerNotFound = NewStratumError(301, "Stratum Server Not Found")
	// StratumErrConnectStratumServerFailed The Stratum Server connection of the corresponding currency failed
	StratumErrConnectStratumServerFailed = NewStratumError(302, "Connect Stratum Server Failed")

	// StratumErrUnknownChainType Unknown blockchain type
	StratumErrUnknownChainType = NewStratumError(500, "Unknown Chain Type")
)

var (
	// ErrReadFailed IO read error
	ErrReadFailed = errors.New("Read Failed")
	// ErrWriteFailed IO write error
	ErrWriteFailed = errors.New("Write Failed")
	// ErrInvalidReader illegal reader
	ErrInvalidReader = errors.New("Invalid Reader")
	// ErrInvalidWritter Illegal Writer
	ErrInvalidWritter = errors.New("Invalid Writter")
	// ErrInvalidBuffer Illegal Buffer
	ErrInvalidBuffer = errors.New("Invalid Buffer")
)
