package model

// BasicResponse mirrors Bark-Notice-Api's response envelope.
type BasicResponse struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

const (
	SuccessCode = "000000"
	ErrorCode   = "999999"
)

// Success wraps data with a success code.
func Success(msg string, data any) BasicResponse {
	return BasicResponse{
		Code: SuccessCode,
		Msg:  msg,
		Data: data,
	}
}

// Error returns a BasicResponse with the default error code.
func Error(msg string) BasicResponse {
	return BasicResponse{
		Code: ErrorCode,
		Msg:  msg,
	}
}

// ErrorWithCode allows specifying a custom error code.
func ErrorWithCode(code, msg string) BasicResponse {
	return BasicResponse{
		Code: code,
		Msg:  msg,
	}
}
