// Code generated by go-swagger; DO NOT EDIT.

// This file is part of MinIO Console Server
// Copyright (c) 2021 MinIO, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//

package admin_api

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"

	"github.com/go-openapi/runtime"

	"github.com/minio/console/models"
)

// ListGroupsForPolicyOKCode is the HTTP code returned for type ListGroupsForPolicyOK
const ListGroupsForPolicyOKCode int = 200

/*ListGroupsForPolicyOK A successful response.

swagger:response listGroupsForPolicyOK
*/
type ListGroupsForPolicyOK struct {

	/*
	  In: Body
	*/
	Payload []string `json:"body,omitempty"`
}

// NewListGroupsForPolicyOK creates ListGroupsForPolicyOK with default headers values
func NewListGroupsForPolicyOK() *ListGroupsForPolicyOK {

	return &ListGroupsForPolicyOK{}
}

// WithPayload adds the payload to the list groups for policy o k response
func (o *ListGroupsForPolicyOK) WithPayload(payload []string) *ListGroupsForPolicyOK {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the list groups for policy o k response
func (o *ListGroupsForPolicyOK) SetPayload(payload []string) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *ListGroupsForPolicyOK) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(200)
	payload := o.Payload
	if payload == nil {
		// return empty array
		payload = make([]string, 0, 50)
	}

	if err := producer.Produce(rw, payload); err != nil {
		panic(err) // let the recovery middleware deal with this
	}
}

/*ListGroupsForPolicyDefault Generic error response.

swagger:response listGroupsForPolicyDefault
*/
type ListGroupsForPolicyDefault struct {
	_statusCode int

	/*
	  In: Body
	*/
	Payload *models.Error `json:"body,omitempty"`
}

// NewListGroupsForPolicyDefault creates ListGroupsForPolicyDefault with default headers values
func NewListGroupsForPolicyDefault(code int) *ListGroupsForPolicyDefault {
	if code <= 0 {
		code = 500
	}

	return &ListGroupsForPolicyDefault{
		_statusCode: code,
	}
}

// WithStatusCode adds the status to the list groups for policy default response
func (o *ListGroupsForPolicyDefault) WithStatusCode(code int) *ListGroupsForPolicyDefault {
	o._statusCode = code
	return o
}

// SetStatusCode sets the status to the list groups for policy default response
func (o *ListGroupsForPolicyDefault) SetStatusCode(code int) {
	o._statusCode = code
}

// WithPayload adds the payload to the list groups for policy default response
func (o *ListGroupsForPolicyDefault) WithPayload(payload *models.Error) *ListGroupsForPolicyDefault {
	o.Payload = payload
	return o
}

// SetPayload sets the payload to the list groups for policy default response
func (o *ListGroupsForPolicyDefault) SetPayload(payload *models.Error) {
	o.Payload = payload
}

// WriteResponse to the client
func (o *ListGroupsForPolicyDefault) WriteResponse(rw http.ResponseWriter, producer runtime.Producer) {

	rw.WriteHeader(o._statusCode)
	if o.Payload != nil {
		payload := o.Payload
		if err := producer.Produce(rw, payload); err != nil {
			panic(err) // let the recovery middleware deal with this
		}
	}
}
