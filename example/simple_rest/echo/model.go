// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package echo

import "encoding/json"

type Request struct {
	Msg string `json:"msg"`
}

func (Request) ContentType() string {
	return "application/json"
}

func (req *Request) UnmarshalBinary(b []byte) error {
	return json.Unmarshal(b, req)
}

type Response struct {
	Msg string `json:"msg"`
}

func (Response) ContentType() string {
	return "application/json"
}

func (resp *Response) MarshalBinary() ([]byte, error) {
	return json.Marshal(resp)
}
