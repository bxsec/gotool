package response

type IResponses interface {
	SetCode(int)
	SetTraceID(int)
	SetMsg(string)
	SetData(interface{})
	SetSuccess(bool)
	Clone() IResponses
}

type Response struct {
	// 数据集
	RequestId int `protobuf:"bytes,1,opt,name=requestId,proto3" json:"requestId,omitempty"`
	Code      int  `protobuf:"varint,2,opt,name=code,proto3" json:"code,omitempty"`
	Msg       string `protobuf:"bytes,3,opt,name=msg,proto3" json:"msg,omitempty"`
	Data interface{} `json:"data"`
	Success bool        `json:"success"`
}


type Page struct {
	Count     int `json:"count"`
	PageIndex int `json:"pageIndex"`
	PageSize  int `json:"pageSize"`
	List interface{} `json:"list"`
}


func (e *Response) SetData(data interface{}) {
	e.Data = data
}

func (e Response) Clone() IResponses {
	return &e
}

func (e *Response) SetTraceID(id int) {
	e.RequestId = id
}

func (e *Response) SetMsg(s string) {
	e.Msg = s
}

func (e *Response) SetCode(code int) {
	e.Code = code
}

func (e *Response) SetSuccess(success bool) {
	e.Success = success
}

func (e *Response) Result(code int, data interface{}, msg string) {
	// 开始时间
	e.Success = false
	if code == SUCCESS {
		e.Success = true
	}
	e.Code = code
	e.Data = data
	e.Msg = msg
}

func (e *Response) Ok() {
	e.Result(SUCCESS, map[string]interface{}{}, "操作成功")
}

func (e *Response) OkWithMessage(message string) {
	e.Result(SUCCESS, map[string]interface{}{}, message)
}

func (e *Response) OkWithData(data interface{}) {
	e.Result(SUCCESS, data, "操作成功")
}

func (e *Response)OkWithDetailed(data interface{}, message string) {
	e.Result(SUCCESS, data, message)
}

func (e *Response)Fail() {
	e.Result(ERROR, map[string]interface{}{}, "操作失败")
}

func (e *Response)FailWithMessage(message string) {
	e.Result(ERROR, map[string]interface{}{}, message)
}

func (e *Response)FailWithDetailed(data interface{}, message string) {
	e.Result(ERROR, data, message)
}

