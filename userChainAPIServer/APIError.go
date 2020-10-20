package main

// APIError API错误结构
type APIError struct {
	// 错误号
	ErrNo int
	// 错误信息
	ErrMsg string
}

// NewAPIError 新建一个APIError
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
	// APIErrPunameIsEmpty puname为空
	APIErrPunameIsEmpty = NewAPIError(101, "puname is empty")
	// APIErrPunameInvalid puname不合法
	APIErrPunameInvalid = NewAPIError(102, "puname invalid")
	// APIErrCoinIsEmpty coin为空
	APIErrCoinIsEmpty = NewAPIError(103, "coin is empty")
	// APIErrCoinIsInexistent coin为空
	APIErrCoinIsInexistent = NewAPIError(104, "coin is inexistent")
	// APIErrReadRecordFailed 读取记录失败
	APIErrReadRecordFailed = NewAPIError(105, "read record failed")

	// APIErrWriteRecordFailed 写入记录失败
	APIErrWriteRecordFailed = NewAPIError(107, "write record failed")

	// APIErrRecordExists 记录已存在
	APIErrRecordExists = NewAPIError(108, "record exists, skip")

	// APIErrUserCoinsEmpty 用户币种数组为空
	APIErrUserCoinsEmpty = NewAPIError(109, "usercoins is empty")

	// APIErrUserSubPoolsEmpty 用户子池数组为空
	APIErrUserSubPoolsEmpty = NewAPIError(110, "usersubpools is empty")
)
