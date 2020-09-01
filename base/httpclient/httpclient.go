package httpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

// 默认超时
const DefTimeout = 60

type RequestWrapper struct {
	url     string
	method  string
	timeout int
	body    io.Reader
	header  map[string]string
}

// 创建一个请求
func NewRequest(url string) *RequestWrapper {
	return &RequestWrapper{url: url}
}

func (r *RequestWrapper) Url(url string) *RequestWrapper {
	r.url = url
	return r
}
func (r *RequestWrapper) Timeout(timeout int) *RequestWrapper {
	r.timeout = timeout
	return r
}

func (r *RequestWrapper) GetByParam(paramMap map[string]string) ResponseWrapper {
	var params string
	for k, v := range paramMap {
		if params != "" {
			params += "&"
		} else {
			params += "?"
		}
		params += k + "=" + v
	}
	r.url += "?" + params
	return r.Get()
}

func (r *RequestWrapper) Get() ResponseWrapper {
	r.method = "GET"
	r.body = nil
	return request(r)
}

func (r *RequestWrapper) PostJson(body string) ResponseWrapper {
	buf := bytes.NewBufferString(body)
	r.method = "POST"
	r.body = buf
	if r.header == nil {
		r.header = make(map[string]string)
	}
	r.header["Content-type"] = "application/json"
	return request(r)
}

func (r *RequestWrapper) PostObj(body interface{}) ResponseWrapper {
	marshal, err := json.Marshal(body)
	if err != nil {
		return createRequestError(errors.New("解析json obj错误"))
	}
	return r.PostJson(string(marshal))
}

func (r *RequestWrapper) PostParams(params string) ResponseWrapper {
	buf := bytes.NewBufferString(params)
	r.method = "POST"
	r.body = buf
	if r.header == nil {
		r.header = make(map[string]string)
	}
	r.header["Content-type"] = "application/x-www-form-urlencoded"
	return request(r)
}

type ResponseWrapper struct {
	StatusCode int
	Body       string
	Header     http.Header
}

func (r *ResponseWrapper) IsSuccess() bool {
	return r.StatusCode == 200
}

func (r *ResponseWrapper) ToObj(obj interface{}) {
	if !r.IsSuccess() {
		return
	}
	_ = json.Unmarshal([]byte(r.Body), &obj)
}

func (r *ResponseWrapper) ToMap() map[string]interface{} {
	if !r.IsSuccess() {
		return nil
	}
	var res map[string]interface{}
	err := json.Unmarshal([]byte(r.Body), &res)
	if err != nil {
		return nil
	}
	return res
}

func request(rw *RequestWrapper) ResponseWrapper {
	wrapper := ResponseWrapper{StatusCode: 0, Body: "", Header: make(http.Header)}
	client := &http.Client{}
	timeout := rw.timeout
	if timeout > 0 {
		client.Timeout = time.Duration(timeout) * time.Second
	} else {
		timeout = DefTimeout
	}

	req, err := http.NewRequest(rw.method, rw.url, rw.body)
	if err != nil {
		return createRequestError(err)
	}
	setRequestHeader(req, rw.header)
	resp, err := client.Do(req)
	if err != nil {
		wrapper.Body = fmt.Sprintf("执行HTTP请求错误-%s", err.Error())
		return wrapper
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		wrapper.Body = fmt.Sprintf("读取HTTP请求返回值失败-%s", err.Error())
		return wrapper
	}
	wrapper.StatusCode = resp.StatusCode
	wrapper.Body = string(body)
	wrapper.Header = resp.Header

	return wrapper
}

func setRequestHeader(req *http.Request, header map[string]string) {
	req.Header.Set("User-Agent", "golang/mayflyjob")
	for k, v := range header {
		req.Header.Set(k, v)
	}
}

func createRequestError(err error) ResponseWrapper {
	errorMessage := fmt.Sprintf("创建HTTP请求错误-%s", err.Error())
	return ResponseWrapper{0, errorMessage, make(http.Header)}
}
