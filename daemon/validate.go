// Copyright 2023 Princess B33f Heavy Industries / Dave Shanley
// SPDX-License-Identifier: AGPL

package daemon

import (
	"github.com/pb33f/libopenapi-validator/errors"
	"github.com/pb33f/ranch/model"
	"net/http"
)

func (ws *WiretapService) ValidateResponse(
	request *model.Request,
	returnedResponse *http.Response) (resultErrors []*errors.ValidationError) {

	defer func() {
		if r := recover(); r != nil {
			// Construct a string from the error and add it to the validation errors.
			resultErrors = append(resultErrors, &errors.ValidationError{
				Message: "Error validating response: " + r.(string),
			})
		}
	}()

	var validationErrors []*errors.ValidationError

	if ws.document != nil && ws.docModel != nil {
		_, validationErrors = ws.validator.ValidateHttpResponse(request.HttpRequest, returnedResponse)
	}

	// wipe out any path not found errors, they are not relevant to the response.
	var cleanedErrors []*errors.ValidationError
	for x := range validationErrors {
		if !validationErrors[x].IsPathMissingError() {
			cleanedErrors = append(cleanedErrors, validationErrors[x])
		}
	}

	transaction := BuildResponse(request, returnedResponse)
	if len(cleanedErrors) > 0 {
		transaction.ResponseValidation = cleanedErrors
	}
	ws.transactionStore.Put(request.Id.String(), transaction, nil)

	if len(cleanedErrors) > 0 {
		ws.streamChan <- cleanedErrors
		ws.broadcastResponseValidationErrors(request, returnedResponse, cleanedErrors)
	} else {
		ws.broadcastResponse(request, returnedResponse)
	}
	return validationErrors
}

func (ws *WiretapService) ValidateRequest(
	modelRequest *model.Request,
	httpRequest *http.Request) (resultErrors []*errors.ValidationError) {

	defer func() {
		if r := recover(); r != nil {
			// Construct a string from the error and add it to the validation errors.
			resultErrors = append(resultErrors, &errors.ValidationError{
				Message: "Error validating request: " + r.(string),
			})
		}
	}()

	var validationErrors, cleanedErrors []*errors.ValidationError

	if ws.document != nil && ws.docModel != nil {
		validator := ws.validator
		_, validationErrors = validator.ValidateHttpRequest(httpRequest)
	}

	for _, validationError := range validationErrors {
		cleanedErrors = append(cleanedErrors, validationError)
	}
	// record results
	buildTransConfig := HttpTransactionConfig{
		OriginalRequest:   modelRequest.HttpRequest,
		NewRequest:        httpRequest,
		ID:                modelRequest.Id,
		TransactionConfig: ws.config,
	}

	transaction := BuildHttpTransaction(buildTransConfig)
	if len(cleanedErrors) > 0 {
		transaction.RequestValidation = cleanedErrors
	}
	ws.transactionStore.Put(modelRequest.Id.String(), modelRequest, nil)

	// broadcast what we found.
	if len(cleanedErrors) > 0 {
		ws.streamChan <- cleanedErrors
		ws.broadcastRequestValidationErrors(modelRequest, cleanedErrors, transaction)
	} else {
		ws.broadcastRequest(modelRequest, transaction)
	}
	return cleanedErrors
}
