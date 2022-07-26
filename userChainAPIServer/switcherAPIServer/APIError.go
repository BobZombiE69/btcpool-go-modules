package switcherapiserver

// APIError API错误结构
type APIError struct {
	// 错误号
	ErrNo int
	// 错误信息
	ErrMsg string
}

// NewAPIError Create a new APIError
func NewAPIError(errNo int, errMsg string) *APIError {
	apiErr := new(APIError)
	apiErr.ErrNo = errNo
	apiErr.ErrMsg = errMsg

	return apiErr
}

func (apiErr *APIError) Error() string {
	return apiErr.ErrMsg
}

var (
	// APIErrPunameIsEmpty puname is empty
	APIErrPunameIsEmpty = NewAPIError(101, "puname is empty")
	// APIErrPunameInvalid puname is illegal
	APIErrPunameInvalid = NewAPIError(102, "puname invalid")
	// APIErrCoinIsEmpty coin is empty
	APIErrCoinIsEmpty = NewAPIError(103, "coin is empty")
	// APIErrCoinIsInexistent coin is empty
	APIErrCoinIsInexistent = NewAPIError(104, "coin is inexistent")
	// APIErrReadRecordFailed Failed to read record
	APIErrReadRecordFailed = NewAPIError(105, "read record failed")

	// APIErrCoinNoChange Currency has not changed
	//(The error no longer occurs, allowing switching to the same currency. This way, if the stratumSwitcher missed the previous switch message, it can receive another switch message to complete the switch)
	//APIErrCoinNoChange = NewAPIError(106, "coin no change")

	// APIErrWriteRecordFailed Failed to write record
	APIErrWriteRecordFailed = NewAPIError(107, "write record failed")

	// APIErrUserCoinsEmpty User currency array is empty
	APIErrUserCoinsEmpty = NewAPIError(108, "usercoins is empty")
)
